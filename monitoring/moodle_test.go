package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
			"id":215841,"cmid":654321,"course":94476,"name":"Example Assignment","duedate":1793242740,
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
	if assignment.ID != 215841 || assignment.CMID != 654321 || assignment.DueDate != 1793242740 || assignment.TeamSubmission != 1 || assignment.NoSubmissions != 0 {
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

func TestDefaultLogLevelIsInfo(t *testing.T) {
	setValidConfigEnvironment(t)
	t.Setenv("LOG_LEVEL", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.logLevel != slog.LevelInfo {
		t.Fatalf("log level = %s, want INFO", cfg.logLevel)
	}
}

func TestValidLogLevels(t *testing.T) {
	tests := []struct {
		value string
		want  slog.Level
	}{
		{value: "debug", want: slog.LevelDebug},
		{value: "info", want: slog.LevelInfo},
		{value: "warn", want: slog.LevelWarn},
		{value: "error", want: slog.LevelError},
		{value: " DEBUG ", want: slog.LevelDebug},
	}

	for _, test := range tests {
		t.Run(strings.TrimSpace(test.value), func(t *testing.T) {
			setValidConfigEnvironment(t)
			t.Setenv("LOG_LEVEL", test.value)
			cfg, err := loadConfig()
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if cfg.logLevel != test.want {
				t.Fatalf("log level = %s, want %s", cfg.logLevel, test.want)
			}
		})
	}
}

func TestInvalidLogLevelIsRejected(t *testing.T) {
	setValidConfigEnvironment(t)
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL must be one of debug, info, warn, error") {
		t.Fatalf("expected clear LOG_LEVEL error, got %v", err)
	}
}

func TestWarningAggregationCountsByOperationAndCode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	summary := newCycleSummary(2)
	summary.recordWarnings(logger, operationAssignments, 10, 0, []moodleWarning{
		{WarningCode: "1", Message: "No access rights in module context"},
		{WarningCode: "1", Message: "No access rights in module context"},
		{WarningCode: "other", Message: "Another warning"},
	})
	summary.recordWarnings(logger, operationSubmissions, 10, 20, []moodleWarning{
		{WarningCode: "1", Message: "Submission unavailable"},
	})

	if summary.warningCount != 4 {
		t.Fatalf("warning count = %d, want 4", summary.warningCount)
	}
	if summary.warningCountsByOperation[operationAssignments] != 3 || summary.warningCountsByOperation[operationSubmissions] != 1 {
		t.Fatalf("unexpected operation counts: %#v", summary.warningCountsByOperation)
	}
	if summary.warningGroups[warningKey{operation: operationAssignments, code: "1"}].count != 2 {
		t.Fatalf("assignment/code warning group was not aggregated: %#v", summary.warningGroups)
	}
	if len(summary.warningGroups) != 3 {
		t.Fatalf("warning groups = %d, want 3", len(summary.warningGroups))
	}
}

func TestAssignmentWarningsMakeCoveragePartial(t *testing.T) {
	summary := newCycleSummary(1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	summary.recordWarnings(logger, operationAssignments, 10, 0, []moodleWarning{{
		WarningCode: "1",
		Message:     "No access rights in module context",
	}})

	if summary.coverage() != "partial" {
		t.Fatalf("coverage = %q, want partial", summary.coverage())
	}
}

func TestUnavailableDataCannotReportCompleteCoverage(t *testing.T) {
	summary := newCycleSummary(1)
	if summary.coverage() != "complete" {
		t.Fatalf("new summary coverage = %q, want complete", summary.coverage())
	}

	student := enrolledUser{ID: 1, Email: "student@example.test", Roles: []moodleRole{{ShortName: "student"}}}
	candidate := assignment{CMID: 201, ID: 200, DueDate: time.Now().Add(time.Hour).Unix()}
	identification := identifyReminderCandidates(100, candidate, []enrolledUser{student}, submissionsResponse{
		Assignments: []assignmentSubmissions{{AssignmentID: candidate.ID}},
	}, "https://moodle.example.test")
	if identification.UnclassifiedStudents == 0 {
		t.Fatal("missing student submission data should be unclassified")
	}
	summary.partialCoverage = identification.UnclassifiedStudents > 0
	if summary.coverage() != "partial" {
		t.Fatalf("unavailable data coverage = %q, want partial", summary.coverage())
	}
}

func TestAggregatedWarningLogEmitsOneRecordPerOperationAndCode(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	summary := newCycleSummary(1)
	summary.recordWarnings(logger, operationAssignments, 10, 0, []moodleWarning{
		{WarningCode: "1", Message: "No access rights in module context"},
		{WarningCode: "1", Message: "No access rights in module context"},
	})
	output.Reset()
	summary.logAggregatedWarnings(logger)

	logs := output.String()
	if strings.Count(logs, "Moodle warnings reduced monitoring coverage") != 1 {
		t.Fatalf("expected one aggregate warning record, got %q", logs)
	}
	if !strings.Contains(logs, "count=2") || !strings.Contains(logs, "operation=get_assignments") {
		t.Fatalf("aggregate warning log omitted counts: %q", logs)
	}
}

func setValidConfigEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("MOODLE_ICAL_URL", "")
	t.Setenv("MOODLE_SOURCE", "auto")
	t.Setenv("NOTIFICATION_EMAIL", "student@example.test")
	t.Setenv("MOODLE_ICAL_EMAIL", "")
	t.Setenv("MOODLE_ICAL_TIMEZONE", "")
	t.Setenv("MOODLE_URL", "https://moodle.example.test")
	t.Setenv("MOODLE_USERNAME", "user")
	t.Setenv("MOODLE_PASSWORD", "password")
	t.Setenv("MOODLE_SERVICE", "")
	t.Setenv("MONITOR_INTERVAL", "")
}

func TestAssignmentEligibility(t *testing.T) {
	now := time.Date(2026, time.October, 30, 12, 0, 0, 0, time.UTC)
	base := assignment{DueDate: now.Add(time.Hour).Unix()}

	tests := []struct {
		name       string
		assignment assignment
		eligible   bool
	}{
		{name: "no deadline", assignment: assignment{}},
		{name: "deadline passed", assignment: assignment{DueDate: now.Add(-time.Second).Unix()}},
		{name: "deadline exactly now", assignment: assignment{DueDate: now.Unix()}},
		{name: "deadline in one hour", assignment: base, eligible: true},
		{name: "deadline exactly 24 hours away", assignment: assignment{DueDate: now.Add(24 * time.Hour).Unix()}, eligible: true},
		{name: "deadline more than 24 hours away", assignment: assignment{DueDate: now.Add(24*time.Hour + time.Second).Unix()}},
		{name: "group assignment", assignment: assignment{DueDate: base.DueDate, TeamSubmission: 1}},
		{name: "submissions disabled", assignment: assignment{DueDate: base.DueDate, NoSubmissions: 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := isAssignmentEligible(test.assignment, now); actual != test.eligible {
				t.Fatalf("isAssignmentEligible() = %t, want %t", actual, test.eligible)
			}
		})
	}
}

func TestIdentifyReminderCandidatesUsesExplicitLatestSubmissionStates(t *testing.T) {
	due := time.Date(2026, time.October, 31, 10, 0, 0, 0, time.UTC)
	candidate := assignment{CMID: 201, ID: 200, Name: "Essay", DueDate: due.Unix()}
	studentRole := []moodleRole{{ID: 5, ShortName: "student", Name: "Student"}}
	users := []enrolledUser{
		{ID: 1, Email: "submitted@example.test", Roles: studentRole},
		{ID: 2, Email: "draft@example.test", Roles: studentRole},
		{ID: 3, Email: "missing@example.test", Roles: studentRole},
		{ID: 4, Email: "teacher@example.test", Roles: []moodleRole{{ID: 3, ShortName: "editingteacher"}}},
		{ID: 5, Roles: studentRole},
		{ID: 6, Email: "reopened@example.test", Roles: studentRole},
	}
	response := submissionsResponse{Assignments: []assignmentSubmissions{{
		AssignmentID: candidate.ID,
		Submissions: []submission{
			{UserID: 1, Status: "submitted", AttemptNumber: 0, Latest: 1},
			{UserID: 2, Status: "draft", AttemptNumber: 0, Latest: 1},
			{UserID: 4, Status: "new", AttemptNumber: 0, Latest: 1},
			{UserID: 5, Status: "new", AttemptNumber: 0, Latest: 1},
			{UserID: 6, Status: "submitted", AttemptNumber: 0},
			{UserID: 6, Status: "reopened", AttemptNumber: 1, Latest: 1},
		},
	}}}

	result := identifyReminderCandidates(100, candidate, users, response, "https://moodle.example.test")
	if !result.SubmissionDataOK {
		t.Fatalf("expected available submission data, got reason %q", result.UnavailableReason)
	}
	if result.CompletedStudents != 1 || result.IncompleteStudents != 3 {
		t.Fatalf("unexpected classification counts: %#v", result)
	}
	if result.UnclassifiedStudents != 1 {
		t.Fatalf("unclassified students = %d, want 1", result.UnclassifiedStudents)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2: %#v", len(result.Candidates), result.Candidates)
	}
	if result.Candidates[0].UserID != 2 || result.Candidates[1].UserID != 6 {
		t.Fatalf("unexpected candidates: %#v", result.Candidates)
	}
	if result.Candidates[0].DueDate != due || result.Candidates[0].CourseID != 100 || result.Candidates[0].AssignmentID != 200 {
		t.Fatalf("candidate metadata was not preserved: %#v", result.Candidates[0])
	}
	if result.Candidates[0].AssignmentURL != "https://moodle.example.test/mod/assign/view.php?id=201" {
		t.Fatalf("assignment link must use cmid, not assignment ID: %q", result.Candidates[0].AssignmentURL)
	}
}

func TestMissingCourseModuleIDDoesNotProduceBrokenLink(t *testing.T) {
	candidate := assignment{ID: 200}
	response := submissionsResponse{Assignments: []assignmentSubmissions{{AssignmentID: 200}}}
	result := identifyReminderCandidates(100, candidate, nil, response, "https://moodle.example.test")
	if result.SubmissionDataOK || len(result.Candidates) != 0 || result.UnavailableReason == "" {
		t.Fatalf("missing cmid must prevent publication: %#v", result)
	}
}

func TestIdentifyReminderCandidatesFailsClosedForWarnings(t *testing.T) {
	student := enrolledUser{ID: 1, Email: "student@example.test", Roles: []moodleRole{{ShortName: "student"}}}
	candidate := assignment{CMID: 201, ID: 200, DueDate: time.Now().Add(time.Hour).Unix()}

	tests := []struct {
		name    string
		warning moodleWarning
	}{
		{
			name:    "permission failure",
			warning: moodleWarning{WarningCode: "nopermissions", Message: "No permission to view submissions"},
		},
		{
			name:    "generic response warning",
			warning: moodleWarning{WarningCode: "unknownwarning", Message: "Submission data may be incomplete"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := submissionsResponse{
				Assignments: []assignmentSubmissions{{
					AssignmentID: candidate.ID,
					Submissions:  []submission{{UserID: student.ID, Status: "draft"}},
				}},
				Warnings: []moodleWarning{test.warning},
			}
			result := identifyReminderCandidates(100, candidate, []enrolledUser{student}, response, "https://moodle.example.test")
			if result.SubmissionDataOK || len(result.Candidates) != 0 {
				t.Fatalf("warning must make submission data unavailable: %#v", result)
			}
		})
	}
}

func TestIdentifyReminderCandidatesDistinguishesEmptyAndMissingResults(t *testing.T) {
	student := enrolledUser{ID: 1, Email: "student@example.test", Roles: []moodleRole{{ShortName: "student"}}}
	candidate := assignment{CMID: 201, ID: 200, DueDate: time.Now().Add(time.Hour).Unix()}

	empty := identifyReminderCandidates(100, candidate, []enrolledUser{student}, submissionsResponse{
		Assignments: []assignmentSubmissions{{AssignmentID: candidate.ID, Submissions: []submission{}}},
	}, "https://moodle.example.test")
	if !empty.SubmissionDataOK || empty.SubmissionCount != 0 || len(empty.Candidates) != 0 {
		t.Fatalf("an explicit empty list should be available but produce no candidates: %#v", empty)
	}

	missing := identifyReminderCandidates(100, candidate, []enrolledUser{student}, submissionsResponse{}, "https://moodle.example.test")
	if missing.SubmissionDataOK || len(missing.Candidates) != 0 {
		t.Fatalf("a missing assignment result must be unavailable: %#v", missing)
	}
}

func TestConflictingRecordsForSameAttemptAreAmbiguous(t *testing.T) {
	student := enrolledUser{ID: 1, Email: "student@example.test", Roles: []moodleRole{{ShortName: "student"}}}
	candidate := assignment{CMID: 201, ID: 200, DueDate: time.Now().Add(time.Hour).Unix()}
	response := submissionsResponse{Assignments: []assignmentSubmissions{{
		AssignmentID: candidate.ID,
		Submissions: []submission{
			{UserID: student.ID, Status: "draft", AttemptNumber: 2},
			{UserID: student.ID, Status: "submitted", AttemptNumber: 2, Latest: 1},
		},
	}}}

	result := identifyReminderCandidates(100, candidate, []enrolledUser{student}, response, "https://moodle.example.test")
	if len(result.Candidates) != 0 || result.CompletedStudents != 0 || result.IncompleteStudents != 0 {
		t.Fatalf("a same-attempt status conflict should remain unclassified: %#v", result)
	}
}

func TestMixedStudentAndStaffRolesAreExcluded(t *testing.T) {
	user := enrolledUser{
		ID:    1,
		Email: "mixed-role@example.test",
		Roles: []moodleRole{{ShortName: "student"}, {ShortName: "editingteacher"}},
	}
	candidate := assignment{CMID: 201, ID: 200, DueDate: time.Now().Add(time.Hour).Unix()}
	response := submissionsResponse{Assignments: []assignmentSubmissions{{
		AssignmentID: candidate.ID,
		Submissions:  []submission{{UserID: user.ID, Status: "draft", Latest: 1}},
	}}}

	result := identifyReminderCandidates(100, candidate, []enrolledUser{user}, response, "https://moodle.example.test")
	if classifyEnrolmentRole(user.Roles) != roleNonStudent || len(result.Candidates) != 0 {
		t.Fatalf("mixed student and staff roles must not produce candidates: %#v", result)
	}
}

func TestMixedStudentAndUnknownRolesAreAmbiguous(t *testing.T) {
	user := enrolledUser{
		ID:    1,
		Email: "mixed-role@example.test",
		Roles: []moodleRole{{ShortName: "student"}, {ShortName: "custom-role"}},
	}
	candidate := assignment{CMID: 201, ID: 200, DueDate: time.Now().Add(time.Hour).Unix()}
	response := submissionsResponse{Assignments: []assignmentSubmissions{{
		AssignmentID: candidate.ID,
		Submissions:  []submission{{UserID: user.ID, Status: "draft", Latest: 1}},
	}}}

	result := identifyReminderCandidates(100, candidate, []enrolledUser{user}, response, "https://moodle.example.test")
	if classifyEnrolmentRole(user.Roles) != roleAmbiguous || len(result.Candidates) != 0 {
		t.Fatalf("mixed student and unknown roles must remain ambiguous: %#v", result)
	}
}

func TestUnknownRolesAreAmbiguousAndExcluded(t *testing.T) {
	users := []enrolledUser{
		{ID: 1, Roles: []moodleRole{{ShortName: "student"}}},
		{ID: 2, Roles: []moodleRole{{ShortName: "editingteacher"}}},
		{ID: 3, Roles: []moodleRole{{ShortName: "customlearner"}}},
		{ID: 4},
	}

	students, ambiguous := countEnrolmentRoles(users)
	if students != 1 || ambiguous != 2 {
		t.Fatalf("role counts = students %d, ambiguous %d; want 1 and 2", students, ambiguous)
	}
	if hasStudentRole(users[2].Roles) || hasStudentRole(users[3].Roles) {
		t.Fatal("ambiguous roles must not be treated as students")
	}
}

func TestGetEnrolledUsersSendsCourseIDAndDecodesRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if request.Form.Get("wsfunction") != "core_enrol_get_enrolled_users" || request.Form.Get("courseid") != "94476" {
			t.Fatalf("unexpected enrolled-users form: %v", request.Form)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"id":42,"fullname":"Example Student","email":"student@example.test","roles":[{"roleid":5,"name":"Student","shortname":"student"}]}]`))
	}))
	defer server.Close()

	client := newMoodleClient(server.URL, "user", "password", "service", server.Client())
	users, err := client.getEnrolledUsers(context.Background(), "token", 94476)
	if err != nil {
		t.Fatalf("get enrolled users: %v", err)
	}
	if len(users) != 1 || users[0].ID != 42 || !hasStudentRole(users[0].Roles) {
		t.Fatalf("unexpected users: %#v", users)
	}
}

func TestGetSubmissionsPermissionFailureReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"exception":"required_capability_exception","errorcode":"nopermissions","message":"No permission to view submissions"}`))
	}))
	defer server.Close()

	client := newMoodleClient(server.URL, "user", "password", "service", server.Client())
	_, err := client.getSubmissions(context.Background(), "token", 200)
	var apiErr *moodleAPIError
	if !errors.As(err, &apiErr) || apiErr.ErrorCode != "nopermissions" {
		t.Fatalf("expected a permission API error, got %v", err)
	}
}
