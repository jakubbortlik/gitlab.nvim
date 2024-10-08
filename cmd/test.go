package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xanzy/go-gitlab"
)

/*
The FakeHandlerClient is used to create a fake gitlab client for testing our handlers, where the gitlab APIs are all mocked depending on what is provided during the variable initialization, so that we can simulate different responses from Gitlab
*/

var errorFromGitlab = errors.New("Some error from Gitlab")

/* The assert function is a helper function used to check two comparables */
func assert[T comparable](t *testing.T, got T, want T) {
	t.Helper()
	if got != want {
		t.Errorf("Got '%v' but wanted '%v'", got, want)
	}
}

/* Will create a new request with the given method, endpoint and body */
func makeRequest(t *testing.T, method string, endpoint string, body any) *http.Request {
	t.Helper()

	var reader io.Reader
	if body != nil {
		j, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(j)
	}

	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatal(err)
	}

	return request
}

/* Serves and parses the JSON from an endpoint into the given type */
func serveRequest[T any](t *testing.T, s *http.ServeMux, request *http.Request, i T) *T {
	t.Helper()
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, request)
	result := recorder.Result()
	decoder := json.NewDecoder(result.Body)
	err := decoder.Decode(&i)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	return &i
}

/* Make response makes a simple response value with the right status code */
func makeResponse(status int) *gitlab.Response {
	return &gitlab.Response{
		Response: &http.Response{
			StatusCode: status,
		},
	}
}

func checkErrorFromGitlab(t *testing.T, data ErrorResponse, msg string) {
	t.Helper()
	assert(t, data.Status, http.StatusInternalServerError)
	assert(t, data.Message, msg)
	assert(t, data.Details, errorFromGitlab.Error())
}

func checkBadMethod(t *testing.T, data ErrorResponse, methods ...string) {
	t.Helper()
	assert(t, data.Status, http.StatusMethodNotAllowed)
	assert(t, data.Details, "Invalid request type")
	expectedMethods := strings.Join(methods, " or ")
	assert(t, data.Message, fmt.Sprintf("Expected %s", expectedMethods))
}

func checkNon200(t *testing.T, data ErrorResponse, msg, endpoint string) {
	t.Helper()
	assert(t, data.Status, http.StatusSeeOther)
	assert(t, data.Message, msg)
	assert(t, data.Details, fmt.Sprintf("An error occurred on the %s endpoint", endpoint))
}
