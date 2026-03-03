package app

import (
	"net/http"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type fakeMergeRequestRebaser struct {
	testBase
}

func (f fakeMergeRequestRebaser) RebaseMergeRequest(pid interface{}, mergeRequest int64, opt *gitlab.RebaseMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Response, error) {
	resp, err := f.handleGitlabError()
	if err != nil {
		return nil, err
	}

	return resp, err
}

func TestRebaseHandler(t *testing.T) {
	var testRebaseMrPayload = RebaseMrRequest{SkipCI: false}
	t.Run("Rebases merge request", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/rebase", testRebaseMrPayload)
		svc := middleware(
			mergeRequestRebaserService{testProjectData, fakeMergeRequestRebaser{}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[RebaseMrRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data := getSuccessData(t, svc, request)
		assert(t, data.Message, "MR rebased successfully")
	})
	var testRebaseMrPayloadSkipCI = RebaseMrRequest{SkipCI: true}
	t.Run("Rebases merge request and skips CI", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/rebase", testRebaseMrPayloadSkipCI)
		svc := middleware(
			mergeRequestRebaserService{testProjectData, fakeMergeRequestRebaser{}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[RebaseMrRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data := getSuccessData(t, svc, request)
		assert(t, data.Message, "MR rebased successfully (skipping CI)")
	})
	t.Run("Handles errors from Gitlab client", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/rebase", testRebaseMrPayload)
		svc := middleware(
			mergeRequestRebaserService{testProjectData, fakeMergeRequestRebaser{testBase{errFromGitlab: true}}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[RebaseMrRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		checkErrorFromGitlab(t, data, "Could not rebase MR")
	})
	t.Run("Handles non-200s from Gitlab", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/rebase", testRebaseMrPayload)
		svc := middleware(
			mergeRequestRebaserService{testProjectData, fakeMergeRequestRebaser{testBase{status: http.StatusSeeOther}}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[RebaseMrRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		checkNon200(t, data, "Could not rebase MR", "/mr/rebase")
	})
}
