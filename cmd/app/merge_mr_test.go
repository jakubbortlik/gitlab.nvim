package app

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

var enumValuesCurrentAndLegacy = []byte(`{
	"data": {
		"__type": {
			"enumValues": [
				{"name": "MERGE_WHEN_CHECKS_PASS"},
				{"name": "MERGE_TRAIN"}
			]
		}
	}
}`)

var enumValuesLegacyOnly = []byte(`{
	"data": {
		"__type": {
			"enumValues": [
				{"name": "MERGE_WHEN_PIPELINE_SUCCEEDS"},
				{"name": "MERGE_TRAIN"}
			]
		}
	}
}`)

var enumValuesUnsupported = []byte(`{
	"data": {
		"__type": {
			"enumValues": [
				{"name": "MERGE_TRAIN"}
			]
		}
	}
}`)

var mergeRequestAcceptSuccess = []byte(`{
	"data": {
		"mergeRequestAccept": {
			"mergeRequest": {"state": "opened", "mergeWhenPipelineSucceeds": true},
			"errors": []
		}
	}
}`)

type fakeMergeRequestAccepter struct {
	testBase
}

func (f fakeMergeRequestAccepter) AcceptMergeRequest(pid interface{}, mergeRequest int64, opt *gitlab.AcceptMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error) {
	resp, err := f.handleGitlabError()
	if err != nil {
		return nil, nil, err
	}

	return &gitlab.MergeRequest{}, resp, err
}

var errorGraphQLNotExpected = errors.New("graphql client should not have been called")

func TestAcceptAndMergeHandler(t *testing.T) {
	var testAcceptMergeRequestPayload = AcceptMergeRequestRequest{Squash: false, SquashMessage: "Squash me!", DeleteBranch: false}
	t.Run("Accepts and merges a merge request", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAcceptMergeRequestPayload)
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{}, fakeGraphQLClient{err: errorGraphQLNotExpected}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data := getSuccessData(t, svc, request)
		assert(t, data.Message, "MR merged successfully")
	})
	t.Run("Handles errors from Gitlab client", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAcceptMergeRequestPayload)
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{testBase{errFromGitlab: true}}, fakeMergeRequestGetter{}, fakeGraphQLClient{err: errorGraphQLNotExpected}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		checkErrorFromGitlab(t, data, "Could not merge MR")
	})
	t.Run("Handles non-200s from Gitlab", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAcceptMergeRequestPayload)
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{testBase{status: http.StatusSeeOther}}, fakeMergeRequestGetter{}, fakeGraphQLClient{err: errorGraphQLNotExpected}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		checkNon200(t, data, "Could not merge MR", "/mr/merge")
	})
}

func TestSetAutoMergeHandler(t *testing.T) {
	var testAutoMergePayload = AcceptMergeRequestRequest{AutoMerge: true, Squash: false, DeleteBranch: false}

	t.Run("Sets the merge request to auto-merge via GraphQL", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAutoMergePayload)
		var captured map[string]any
		client := fakeGraphQLClient{
			introspectionData: enumValuesCurrentAndLegacy,
			jsonData:          mergeRequestAcceptSuccess,
			capturedVariables: &captured,
		}
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{}, client},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data := getSuccessData(t, svc, request)
		assert(t, data.Message, "MR set to be merged when all checks pass")
		assert(t, captured["strategy"].(string), "MERGE_WHEN_CHECKS_PASS")
	})

	t.Run("Falls back to the legacy strategy name on older GitLab instances", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAutoMergePayload)
		var captured map[string]any
		client := fakeGraphQLClient{
			introspectionData: enumValuesLegacyOnly,
			jsonData:          mergeRequestAcceptSuccess,
			capturedVariables: &captured,
		}
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{}, client},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data := getSuccessData(t, svc, request)
		assert(t, data.Message, "MR set to be merged when all checks pass")
		assert(t, captured["strategy"].(string), "MERGE_WHEN_PIPELINE_SUCCEEDS")
	})

	t.Run("Fails when the instance supports none of the known strategy names", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAutoMergePayload)
		client := fakeGraphQLClient{
			introspectionData: enumValuesUnsupported,
		}
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{}, client},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		assert(t, data.Message, "Could not set MR to auto-merge")
		if !strings.Contains(data.Details, "none of the supported auto-merge strategies") {
			t.Errorf("expected details to mention unsupported strategies, got %q", data.Details)
		}
	})

	t.Run("Handles business errors returned by the GraphQL mutation", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAutoMergePayload)
		client := fakeGraphQLClient{
			introspectionData: enumValuesCurrentAndLegacy,
			jsonData: []byte(`{
				"data": {
					"mergeRequestAccept": {
						"mergeRequest": {"state": "opened", "mergeWhenPipelineSucceeds": false},
						"errors": ["Sha does not match HEAD of source branch"]
					}
				}
			}`),
		}
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{}, client},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, status := getFailData(t, svc, request)
		assert(t, data.Message, "Could not set MR to auto-merge")
		assert(t, data.Details, "Sha does not match HEAD of source branch")
		assert(t, status, http.StatusUnprocessableEntity)
	})

	t.Run("Handles transport errors from the GraphQL client", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAutoMergePayload)
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{}, fakeGraphQLClient{err: errorFromGitlab}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		assert(t, data.Message, "Could not set MR to auto-merge")
		if !strings.Contains(data.Details, errorFromGitlab.Error()) {
			t.Errorf("expected details to contain %q, got %q", errorFromGitlab.Error(), data.Details)
		}
	})

	t.Run("Fails before calling GraphQL if fetching the sha fails", func(t *testing.T) {
		request := makeRequest(t, http.MethodPost, "/mr/merge", testAutoMergePayload)
		svc := middleware(
			mergeRequestAccepterService{testProjectData, fakeMergeRequestAccepter{}, fakeMergeRequestGetter{testBase{errFromGitlab: true}}, fakeGraphQLClient{err: errorGraphQLNotExpected}},
			withMr(testProjectData, fakeMergeRequestLister{}),
			withPayloadValidation(methodToPayload{
				http.MethodPost: newPayload[AcceptMergeRequestRequest],
			}),
			withMethodCheck(http.MethodPost),
		)
		data, _ := getFailData(t, svc, request)
		checkErrorFromGitlab(t, data, "Could not set MR to auto-merge")
	})
}
