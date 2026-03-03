package app

import (
	"encoding/json"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type RebaseMrRequest struct {
	SkipCI bool `json:"skip_ci,omitempty"`
}

type MergeRequestRebaser interface {
	RebaseMergeRequest(pid interface{}, mergeRequest int64, opt *gitlab.RebaseMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Response, error)
}

type mergeRequestRebaserService struct {
	data
	client MergeRequestRebaser
}

/* Rebases a merge request on the server */
func (a mergeRequestRebaserService) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	payload := r.Context().Value(payload("payload")).(*RebaseMrRequest)

	opts := gitlab.RebaseMergeRequestOptions{
		SkipCI: &payload.SkipCI,
	}

	res, err := a.client.RebaseMergeRequest(a.projectInfo.ProjectId, a.projectInfo.MergeId, &opts)

	if err != nil {
		handleError(w, err, "Could not rebase MR", http.StatusInternalServerError)
		return
	}

	if res.StatusCode >= 300 {
		handleError(w, GenericError{r.URL.Path}, "Could not rebase MR", res.StatusCode)
		return
	}

	skippingCI := ""
	if payload.SkipCI {
		skippingCI = " (skipping CI)"
	}
	response := SuccessResponse{Message: fmt.Sprintf("MR rebased successfully%s", skippingCI)}

	w.WriteHeader(http.StatusOK)

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		handleError(w, err, "Could not encode response", http.StatusInternalServerError)
	}
}
