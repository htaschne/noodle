package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMissingSubmissionRequiresIndividualConfirmation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		pending bool
	}{
		{"never started", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true},"warnings":[]}`, true},
		{"draft", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"submission":{"userid":42,"status":"draft"}}}`, true},
		{"submitted since bulk request", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"submission":{"userid":42,"status":"submitted"}}}`, false},
		{"unknown status", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"submission":{"userid":42,"status":"unknown"}}}`, false},
		{"wrong student", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"submission":{"userid":99,"status":"new"}}}`, false},
		{"permission failure", `{"exception":"required_capability_exception","errorcode":"nopermissions"}`, false},
		{"warning", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true},"warnings":[{"warningcode":"1"}]}`, false},
		{"missing attempt", `{"warnings":[]}`, false},
		{"missing permissions", `{"lastattempt":{"submissionsenabled":true}}`, false},
		{"disabled", `{"lastattempt":{"submissionsenabled":false,"caneditowner":true}}`, false},
		{"locked", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":true}}`, false},
		{"individual extension", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"extensionduedate":1795186800}}`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				if err := request.ParseForm(); err != nil {
					t.Error(err)
				}
				if request.Method != http.MethodPost || request.URL.Path != "/webservice/rest/server.php" ||
					request.Form.Get("wsfunction") != "mod_assign_get_submission_status" ||
					request.Form.Get("assignid") != "200" || request.Form.Get("userid") != "42" ||
					request.Form.Get("wstoken") != "test-token" {
					t.Error("unexpected submission status request")
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			client := newMoodleClient(server.URL, "user", "password", "test", server.Client())
			users := []enrolledUser{
				{ID: 42, Email: "student@example.test", Roles: []moodleRole{{ShortName: "student"}}},
				{ID: 43, Email: "submitted@example.test", Roles: []moodleRole{{ShortName: "student"}}},
				{ID: 44, Email: "teacher@example.test", Roles: []moodleRole{{ShortName: "teacher"}}},
				{ID: 45, Roles: []moodleRole{{ShortName: "student"}}},
			}
			response := submissionsResponse{Assignments: []assignmentSubmissions{{
				AssignmentID: 200, Submissions: []submission{{UserID: 43, Status: "submitted"}},
			}}}
			confirmed, err := confirmMissingSubmissions(context.Background(), client, "test-token", 200, users, response, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			result := identifyReminderCandidates(100, assignment{ID: 200, CMID: 201}, users, confirmed, server.URL)
			if (len(result.Candidates) == 1) != test.pending || len(result.Candidates) > 1 {
				t.Fatalf("pending=%t, candidates=%#v", test.pending, result.Candidates)
			}
			if test.pending && result.Candidates[0].UserID != 42 {
				t.Fatalf("unexpected recipient: %#v", result.Candidates)
			}
			if calls != 1 {
				t.Fatalf("status requests=%d, want only the missing student", calls)
			}
		})
	}
}

func TestEmptyAssignmentWarningStillRequiresIndividualConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		warning   moodleWarning
		confirmed bool
	}{
		{"no attempts", moodleWarning{Item: "module", ItemID: 200, WarningCode: "3"}, true},
		{"another assignment", moodleWarning{Item: "module", ItemID: 999, WarningCode: "3"}, false},
		{"different warning", moodleWarning{Item: "module", ItemID: 200, WarningCode: "1"}, false},
		{"different item", moodleWarning{Item: "course", ItemID: 200, WarningCode: "3"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls++
				_, _ = writer.Write([]byte(`{"lastattempt":{"submissionsenabled":true,"caneditowner":true}}`))
			}))
			defer server.Close()
			client := newMoodleClient(server.URL, "user", "password", "test", server.Client())
			users := []enrolledUser{{ID: 42, Email: "student@example.test", Roles: []moodleRole{{ShortName: "student"}}}}
			response := submissionsResponse{
				Assignments: []assignmentSubmissions{{AssignmentID: 200, Submissions: []submission{}}},
				Warnings:    []moodleWarning{test.warning},
			}
			confirmed, err := confirmMissingSubmissions(context.Background(), client, "test-token", 200, users, response, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			result := identifyReminderCandidates(100, assignment{ID: 200, CMID: 201}, users, confirmed, server.URL)
			if (len(result.Candidates) == 1) != test.confirmed || (calls == 1) != test.confirmed {
				t.Fatalf("confirmed=%t, candidates=%#v, requests=%d", test.confirmed, result.Candidates, calls)
			}
		})
	}
}
