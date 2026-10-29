package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthenticateReturnsOnlyAuthenticationToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/login/token.php" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("unexpected content type: %s", request.Header.Get("Content-Type"))
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if request.Form.Get("username") != "student" || request.Form.Get("password") != "password" || request.Form.Get("service") != "moodle_mobile_app" {
			t.Fatal("authentication form did not contain the expected values")
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"token":"authentication-token","privatetoken":"unused-private-token"}`))
	}))
	defer server.Close()

	client := newMoodleClient(server.URL, "student", "password", "moodle_mobile_app", server.Client())
	token, err := client.authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if token != "authentication-token" {
		t.Fatalf("unexpected token: %q", token)
	}
}

func TestMoodleAPIErrorIsDetectedAndRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"exception":"invalid_parameter_exception","errorcode":"invalidparameter","message":"student password secret-token"}`))
	}))
	defer server.Close()

	client := newMoodleClient(server.URL, "student", "password", "moodle_mobile_app", server.Client())
	_, err := client.getSiteInfo(context.Background(), "secret-token")
	if err == nil {
		t.Fatal("expected a Moodle API error")
	}

	var apiErr *moodleAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected moodleAPIError, got %T", err)
	}
	if apiErr.ErrorCode != "invalidparameter" || apiErr.Exception != "invalid_parameter_exception" {
		t.Fatalf("unexpected API error: %#v", apiErr)
	}
	for _, secret := range []string{"student", "password", "secret-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked secret %q", secret)
		}
	}
}

func TestAssignmentAndSubmissionResponsesDecode(t *testing.T) {
	assignmentJSON := []byte(`{
		"courses":[{"id":94476,"fullname":"Example Course","assignments":[{
			"id":215841,"course":94476,"name":"Example Assignment","duedate":1793242740,
			"teamsubmission":1,"nosubmissions":0
		}]}],
		"warnings":[{"warningcode":"sample-warning","message":"Sample warning"}]
	}`)
	var assignments assignmentsResponse
	if err := json.Unmarshal(assignmentJSON, &assignments); err != nil {
		t.Fatalf("decode assignments: %v", err)
	}
	if len(assignments.Courses) != 1 || len(assignments.Courses[0].Assignments) != 1 {
		t.Fatalf("unexpected assignment response: %#v", assignments)
	}
	assignment := assignments.Courses[0].Assignments[0]
	if assignment.ID != 215841 || assignment.DueDate != 1793242740 || assignment.TeamSubmission != 1 || assignment.NoSubmissions != 0 {
		t.Fatalf("unexpected assignment: %#v", assignment)
	}

	submissionJSON := []byte(`{
		"assignments":[{"assignmentid":215841,"submissions":[{"status":"submitted"},{"status":"new"}]}],
		"warnings":[]
	}`)
	var submissions submissionsResponse
	if err := json.Unmarshal(submissionJSON, &submissions); err != nil {
		t.Fatalf("decode submissions: %v", err)
	}
	count, statuses := summarizeSubmissions(submissions)
	if count != 2 || statuses != "new=1,submitted=1" {
		t.Fatalf("unexpected submission summary: count=%d statuses=%q", count, statuses)
	}
}
