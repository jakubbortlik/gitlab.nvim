package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type AcceptMergeRequestRequest struct {
	AutoMerge     bool   `json:"auto_merge"`
	DeleteBranch  bool   `json:"delete_branch"`
	SquashMessage string `json:"squash_message"`
	Squash        bool   `json:"squash"`
}

type MergeRequestAccepter interface {
	AcceptMergeRequest(pid interface{}, mergeRequest int64, opt *gitlab.AcceptMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
}

type mergeRequestAccepterService struct {
	data
	client        MergeRequestAccepter
	mrGetter      MergeRequestGetter
	graphqlClient gitlab.GraphQLInterface
}

/*
	acceptAndMergeHandler merges a given merge request into the target branch, or,

when auto_merge is requested, sets it to merge once the pipeline/checks pass via
GraphQL (the REST auto_merge option fails to take effect while the pipeline is
still running)
*/
func (a mergeRequestAccepterService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload := r.Context().Value(payload("payload")).(*AcceptMergeRequestRequest)

	if payload.AutoMerge {
		a.setAutoMerge(w, r, payload)
		return
	}

	opts := gitlab.AcceptMergeRequestOptions{
		AutoMerge:                &payload.AutoMerge,
		Squash:                   &payload.Squash,
		ShouldRemoveSourceBranch: &payload.DeleteBranch,
	}

	if payload.SquashMessage != "" {
		opts.SquashCommitMessage = &payload.SquashMessage
	}

	_, res, err := a.client.AcceptMergeRequest(a.projectInfo.ProjectId, a.projectInfo.MergeId, &opts)

	if err != nil {
		handleError(w, err, "Could not merge MR", http.StatusInternalServerError)
		return
	}

	if res.StatusCode >= 300 {
		handleError(w, GenericError{r.URL.Path}, "Could not merge MR", res.StatusCode)
		return
	}

	response := SuccessResponse{Message: "MR merged successfully"}

	w.WriteHeader(http.StatusOK)

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		handleError(w, err, "Could not encode response", http.StatusInternalServerError)
	}
}

const mergeRequestAcceptMutation = `
mutation AcceptMergeRequest($projectPath: ID!, $iid: String!, $sha: String!, $strategy: MergeStrategyEnum, $squash: Boolean, $squashCommitMessage: String, $shouldRemoveSourceBranch: Boolean) {
	mergeRequestAccept(input: {
		projectPath: $projectPath,
		iid: $iid,
		sha: $sha,
		strategy: $strategy,
		squash: $squash,
		squashCommitMessage: $squashCommitMessage,
		shouldRemoveSourceBranch: $shouldRemoveSourceBranch
	}) {
		mergeRequest {
			state
			mergeWhenPipelineSucceeds
		}
		errors
	}
}
`

// graphQLErrors captures the top-level "errors" array GraphQL returns (with an HTTP
// 200) for request-level failures like an unknown field/enum value, as opposed to the
// business-level errors returned inside a mutation's own payload.
type graphQLErrors struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (e graphQLErrors) firstError() (string, bool) {
	if len(e.Errors) == 0 {
		return "", false
	}
	return e.Errors[0].Message, true
}

type mergeRequestAcceptGraphQLResponse struct {
	graphQLErrors
	Data struct {
		MergeRequestAccept struct {
			MergeRequest struct {
				State                     string `json:"state"`
				MergeWhenPipelineSucceeds bool   `json:"mergeWhenPipelineSucceeds"`
			} `json:"mergeRequest"`
			Errors []string `json:"errors"`
		} `json:"mergeRequestAccept"`
	} `json:"data"`
}

const mergeStrategyEnumQuery = `
query GetMergeStrategyEnumValues {
	__type(name: "MergeStrategyEnum") {
		enumValues {
			name
		}
	}
}
`

type mergeStrategyEnumResponse struct {
	graphQLErrors
	Data struct {
		Type *struct {
			EnumValues []struct {
				Name string `json:"name"`
			} `json:"enumValues"`
		} `json:"__type"`
	} `json:"data"`
}

// autoMergeStrategyCandidates lists the GraphQL MergeStrategyEnum value that means
// "merge once the pipeline/required checks succeed", newest name first. GitLab has
// renamed this strategy across releases (older self-managed instances only expose
// MERGE_WHEN_PIPELINE_SUCCEEDS), so the schema is introspected at request time to find
// whichever name the target instance actually supports, rather than assuming a version.
var autoMergeStrategyCandidates = []string{
	"MERGE_WHEN_CHECKS_PASS",
	"MERGE_WHEN_PIPELINE_SUCCEEDS",
}

/*
	resolveAutoMergeStrategy looks up which MergeStrategyEnum value the target GitLab

instance's GraphQL schema supports for "merge once checks pass"
*/
func (a mergeRequestAccepterService) resolveAutoMergeStrategy() (string, error) {
	var response mergeStrategyEnumResponse
	_, err := a.graphqlClient.Do(gitlab.GraphQLQuery{Query: mergeStrategyEnumQuery}, &response)
	if err != nil {
		return "", fmt.Errorf("failed to introspect MergeStrategyEnum: %w", err)
	}

	if msg, ok := response.firstError(); ok {
		return "", fmt.Errorf("failed to introspect MergeStrategyEnum: %s", msg)
	}

	if response.Data.Type == nil {
		return "", errors.New("this GitLab instance's GraphQL schema does not define MergeStrategyEnum")
	}

	available := make(map[string]bool, len(response.Data.Type.EnumValues))
	for _, v := range response.Data.Type.EnumValues {
		available[v.Name] = true
	}

	for _, candidate := range autoMergeStrategyCandidates {
		if available[candidate] {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("none of the supported auto-merge strategies (%s) are available on this GitLab instance", strings.Join(autoMergeStrategyCandidates, ", "))
}

/*
	setAutoMerge fetches the merge request's current diff HEAD sha via REST and then

sets the merge request to auto-merge via the GraphQL mergeRequestAccept mutation,
which (unlike the REST accept endpoint) succeeds while the pipeline is running
*/
func (a mergeRequestAccepterService) setAutoMerge(w http.ResponseWriter, r *http.Request, payload *AcceptMergeRequestRequest) {
	mr, res, err := a.mrGetter.GetMergeRequest(a.projectInfo.ProjectId, a.projectInfo.MergeId, &gitlab.GetMergeRequestsOptions{})
	if err != nil {
		handleError(w, err, "Could not set MR to auto-merge", http.StatusInternalServerError)
		return
	}

	if res.StatusCode >= 300 {
		handleError(w, GenericError{r.URL.Path}, "Could not set MR to auto-merge", res.StatusCode)
		return
	}

	strategy, err := a.resolveAutoMergeStrategy()
	if err != nil {
		handleError(w, err, "Could not set MR to auto-merge", http.StatusInternalServerError)
		return
	}

	variables := map[string]any{
		"projectPath":              a.gitInfo.ProjectPath(),
		"iid":                      fmt.Sprintf("%d", a.projectInfo.MergeId),
		"sha":                      mr.DiffRefs.HeadSha,
		"strategy":                 strategy,
		"squash":                   payload.Squash,
		"shouldRemoveSourceBranch": payload.DeleteBranch,
	}
	if payload.SquashMessage != "" {
		variables["squashCommitMessage"] = payload.SquashMessage
	}

	var response mergeRequestAcceptGraphQLResponse
	_, err = a.graphqlClient.Do(gitlab.GraphQLQuery{
		Query:     mergeRequestAcceptMutation,
		Variables: variables,
	}, &response)
	if err != nil {
		handleError(w, err, "Could not set MR to auto-merge", http.StatusInternalServerError)
		return
	}

	if msg, ok := response.firstError(); ok {
		handleError(w, errors.New(msg), "Could not set MR to auto-merge", http.StatusInternalServerError)
		return
	}

	if len(response.Data.MergeRequestAccept.Errors) > 0 {
		err := fmt.Errorf("%s", response.Data.MergeRequestAccept.Errors[0])
		handleError(w, err, "Could not set MR to auto-merge", http.StatusUnprocessableEntity)
		return
	}

	successResponse := SuccessResponse{Message: "MR set to be merged when all checks pass"}

	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(successResponse); err != nil {
		handleError(w, err, "Could not encode response", http.StatusInternalServerError)
	}
}
