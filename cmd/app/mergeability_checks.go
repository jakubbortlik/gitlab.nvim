package app

import (
	"encoding/json"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type MergeabilityCheck struct {
	Identifier string `json:"identifier"`
	Status     string `json:"status"`
}

type MergeabilityChecksResponse struct {
	SuccessResponse
	MergeabilityChecks []*MergeabilityCheck `json:"mergeability_checks"`
}

type mergeabilityChecksGraphQLResponse struct {
	Data struct {
		Project struct {
			MergeRequest struct {
				MergeabilityChecks []*MergeabilityCheck `json:"mergeabilityChecks"`
			} `json:"mergeRequest"`
		} `json:"project"`
	} `json:"data"`
}

const mergeabilityChecksQuery = `
query GetMergeabilityChecks($projectPath: ID!, $iid: String!) {
	project(fullPath: $projectPath) {
		mergeRequest(iid: $iid) {
			mergeabilityChecks {
				identifier
				status
			}
		}
	}
}
`

type mergeabilityChecksService struct {
	data
	client gitlab.GraphQLInterface
}

func (a mergeabilityChecksService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	checks, err := a.fetchMergeabilityChecks()
	if err != nil {
		handleError(w, err, "Could not get mergeability checks", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	response := MergeabilityChecksResponse{
		SuccessResponse:    SuccessResponse{Message: "Mergeability checks retrieved"},
		MergeabilityChecks: checks,
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		handleError(w, err, "Could not encode response", http.StatusInternalServerError)
	}
}

func (a mergeabilityChecksService) fetchMergeabilityChecks() ([]*MergeabilityCheck, error) {
	var response mergeabilityChecksGraphQLResponse

	_, err := a.client.Do(gitlab.GraphQLQuery{
		Query: mergeabilityChecksQuery,
		Variables: map[string]any{
			"projectPath": a.gitInfo.ProjectPath(),
			"iid":         fmt.Sprintf("%d", a.projectInfo.MergeId),
		},
	}, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mergeability checks: %w", err)
	}

	return response.Data.Project.MergeRequest.MergeabilityChecks, nil
}
