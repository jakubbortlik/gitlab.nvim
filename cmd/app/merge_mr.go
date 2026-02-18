package app

import (
	"encoding/json"
	"net/http"

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
	client MergeRequestAccepter
}

/* acceptAndMergeHandler merges a given merge request into the target branch */
func (a mergeRequestAccepterService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload := r.Context().Value(payload("payload")).(*AcceptMergeRequestRequest)

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

	var message string
	if payload.AutoMerge {
		message = "MR set to be merged when all checks pass"
	} else {
		message = "MR merged successfully"
	}
	response := SuccessResponse{Message: message}

	w.WriteHeader(http.StatusOK)

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		handleError(w, err, "Could not encode response", http.StatusInternalServerError)
	}
}
