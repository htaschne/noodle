package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func pendingOwnStatus(t *testing.T, record string) submissionStatusResponse {
	t.Helper()
	var response submissionStatusResponse
	if err := json.Unmarshal([]byte(`{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":false,"graded":false,"extensionduedate":0`+record+`},"warnings":[]}`), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestPersonalSubmissionClassification(t *testing.T) {
	candidate := assignment{ID: 200, CMID: 201, Name: "Essay", DueDate: time.Now().Add(time.Hour).Unix()}
	for _, test := range []struct {
		name, record       string
		pending, available bool
	}{
		{"never started", "", true, true},
		{"new", `,"submission":{"userid":42,"status":"new"}`, true, true},
		{"draft", `,"submission":{"userid":42,"status":"draft"}`, true, true},
		{"reopened", `,"submission":{"userid":42,"status":"reopened"}`, true, true},
		{"submitted", `,"submission":{"userid":42,"status":"submitted"}`, false, true},
		{"unknown", `,"submission":{"userid":42,"status":"unknown"}`, false, false},
		{"another student", `,"submission":{"userid":99,"status":"new"}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := identifyPersonalReminder(100, candidate, 42, "student@example.test", pendingOwnStatus(t, test.record), "https://moodle.example.test")
			if (len(result.Candidates) == 1) != test.pending || result.SubmissionDataOK != test.available {
				t.Fatalf("unexpected classification: %#v", result)
			}
			if test.pending && (result.Candidates[0].UserID != 42 || result.Candidates[0].AssignmentURL != "https://moodle.example.test/mod/assign/view.php?id=201") {
				t.Fatalf("incorrect owner or link: %#v", result.Candidates)
			}
		})
	}
}

func TestPersonalNullExtensionDate(t *testing.T) {
	var status submissionStatusResponse
	if err := json.Unmarshal([]byte(`{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":false,"graded":false,"extensionduedate":null,"submission":{"userid":42,"status":"new"}}}`), &status); err != nil {
		t.Fatal(err)
	}
	candidate := assignment{ID: 200, CMID: 201, DueDate: time.Now().Add(time.Hour).Unix()}
	result := identifyPersonalReminder(100, candidate, 42, "student@example.test", status, "https://moodle.example.test")
	if !result.SubmissionDataOK || len(result.Candidates) != 1 {
		t.Fatalf("null no-extension date rejected: %#v", result)
	}
}

func TestPersonalStatusFailsClosed(t *testing.T) {
	candidate := assignment{ID: 200, CMID: 201}
	for _, test := range []struct{ name, body string }{
		{"empty response", `{}`},
		{"empty attempt", `{"lastattempt":{}}`},
		{"warning", `{"lastattempt":{},"warnings":[{"warningcode":"nopermission"}]}`},
		{"locked", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":true,"graded":false,"extensionduedate":0}}`},
		{"disabled", `{"lastattempt":{"submissionsenabled":false,"caneditowner":true,"locked":false,"graded":false,"extensionduedate":0}}`},
		{"owner cannot edit", `{"lastattempt":{"submissionsenabled":true,"caneditowner":false,"locked":false,"graded":false,"extensionduedate":0}}`},
		{"extension", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":false,"graded":false,"extensionduedate":123}}`},
		{"graded offline", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":false,"graded":true,"extensionduedate":0}}`},
		{"team submission", `{"lastattempt":{"submissionsenabled":true,"caneditowner":true,"locked":false,"graded":false,"extensionduedate":0,"teamsubmission":{"userid":0,"status":"new"}}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var status submissionStatusResponse
			if err := json.Unmarshal([]byte(test.body), &status); err != nil {
				t.Fatal(err)
			}
			if result := identifyPersonalReminder(100, candidate, 42, "student@example.test", status, "https://moodle.example.test"); len(result.Candidates) != 0 {
				t.Fatalf("unsafe candidate: %#v", result)
			}
		})
	}
}

// These are fictional Moodle REST responses. The server rejects roster and bulk
// submission requests, so a passing cycle demonstrates student-level access.
func personalMoodleServer(t *testing.T, assignments []assignment, statuses map[int64]submissionStatusResponse, failure *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("Moodle requests must use POST")
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/login/token.php" {
			fmt.Fprint(w, `{"token":"fictional-token"}`)
			return
		}
		if r.Form.Get("wstoken") != "fictional-token" {
			t.Error("missing API token")
		}
		switch r.Form.Get("wsfunction") {
		case "core_webservice_get_site_info":
			fmt.Fprint(w, `{"userid":42}`)
		case "core_enrol_get_users_courses":
			if r.Form.Get("userid") != "42" {
				t.Error("requested another user's courses")
			}
			fmt.Fprint(w, `[{"id":100,"fullname":"Fictional course"}]`)
		case "mod_assign_get_assignments":
			json.NewEncoder(w).Encode(assignmentsResponse{Courses: []assignmentCourse{{ID: 100, Assignments: assignments}}})
		case "mod_assign_get_submission_status":
			if r.Form.Has("userid") {
				t.Error("personal status must use the current-user default")
			}
			if failure != nil && *failure {
				fmt.Fprint(w, `{"exception":"required_capability_exception","errorcode":"nopermission","message":"fictional-token student-password"}`)
				return
			}
			var id int64
			fmt.Sscan(r.Form.Get("assignid"), &id)
			status, ok := statuses[id]
			if !ok {
				t.Errorf("unexpected status query for assignment %d", id)
			}
			json.NewEncoder(w).Encode(status)
		default:
			t.Errorf("unexpected/class-wide function: %s", r.Form.Get("wsfunction"))
			fmt.Fprint(w, `{"exception":"required_capability_exception","errorcode":"nopermission"}`)
		}
	}))
}

func TestPersonalMonitoringCycle(t *testing.T) {
	now := time.Now()
	assignments := []assignment{
		{ID: 1, CMID: 101, Name: "Never started", DueDate: now.Add(time.Hour).Unix()},
		{ID: 2, CMID: 102, Name: "Draft", DueDate: now.Add(time.Hour).Unix()},
		{ID: 3, CMID: 103, Name: "Submitted", DueDate: now.Add(time.Hour).Unix()},
		{ID: 4, CMID: 104, Name: "Outside window", DueDate: now.Add(48 * time.Hour).Unix()},
		{ID: 5, CMID: 105, Name: "Overdue", DueDate: now.Add(-time.Hour).Unix()},
		{ID: 6, CMID: 106, Name: "Group", DueDate: now.Add(time.Hour).Unix(), TeamSubmission: 1},
		{ID: 7, CMID: 107, Name: "No deadline"},
	}
	statuses := map[int64]submissionStatusResponse{1: pendingOwnStatus(t, ""), 2: pendingOwnStatus(t, `,"submission":{"userid":42,"status":"draft"}`), 3: pendingOwnStatus(t, `,"submission":{"userid":42,"status":"submitted"}`)}
	server := personalMoodleServer(t, assignments, statuses, nil)
	defer server.Close()
	client := newMoodleClient(server.URL, "student", "student-password", "service", server.Client())
	client.email = "student@example.test"
	pub := &recordingReminderPublisher{}
	factory := &recordingPublisherFactory{publisher: pub}
	client.newPublisher = factory.New
	if err := monitorCycle(context.Background(), client, discardLogger()); err != nil {
		t.Fatal(err)
	}
	if len(pub.messages) != 2 || pub.messages[0].ID != "100:1:42" || pub.messages[1].ID != "100:2:42" {
		t.Fatalf("incorrect personal reminders: %#v", pub.messages)
	}
	if err := monitorCycle(context.Background(), client, discardLogger()); err != nil {
		t.Fatal(err)
	}
	if len(pub.messages) != 4 || pub.messages[0].ID != pub.messages[2].ID {
		t.Fatal("polling changed the persisted deduplication identity")
	}
}

func TestPersonalStatusFailureIsSafeAndRecovers(t *testing.T) {
	failed := true
	server := personalMoodleServer(t, []assignment{{ID: 1, CMID: 101, Name: "Essay", DueDate: time.Now().Add(time.Hour).Unix()}}, map[int64]submissionStatusResponse{1: pendingOwnStatus(t, "")}, &failed)
	defer server.Close()
	client := newMoodleClient(server.URL, "student", "student-password", "service", server.Client())
	client.email = "student@example.test"
	factory := &recordingPublisherFactory{publisher: &recordingReminderPublisher{}}
	client.newPublisher = factory.New
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	if err := monitorCycle(context.Background(), client, logger); err != nil {
		t.Fatal(err)
	}
	if factory.calls != 0 || !strings.Contains(logs.String(), "coverage=partial") {
		t.Fatalf("failure did not skip publication safely: %s", logs.String())
	}
	for _, secret := range []string{"fictional-token", "student-password"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("secret leaked in error log")
		}
	}
	failed = false
	if err := monitorCycle(context.Background(), client, discardLogger()); err != nil || len(factory.publisher.messages) != 1 {
		t.Fatalf("did not recover: %v", err)
	}
}

func TestExplicitPersonalSourcePreservesCalendarConfiguration(t *testing.T) {
	setValidConfigEnvironment(t)
	t.Setenv("MOODLE_SOURCE", "api")
	t.Setenv("MOODLE_ICAL_URL", "https://moodle.example.test/calendar/export_execute.php?authtoken=SECRET")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := newMonitoringSource(cfg, http.DefaultClient).(*moodleClient); !ok {
		t.Fatal("explicit API mode did not override the saved calendar URL")
	}
	if cfg.moodleICalURL == "" {
		t.Fatal("calendar configuration was discarded")
	}
	t.Setenv("NOTIFICATION_EMAIL", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("personal API mode accepted a missing recipient")
	}
	t.Setenv("MOODLE_SOURCE", "unknown")
	if _, err := loadConfig(); err == nil {
		t.Fatal("invalid source accepted")
	}
}

// This opt-in live test is read-only and never creates a RabbitMQ publisher.
func TestLivePersonalMoodleIntegration(t *testing.T) {
	if os.Getenv("TEST_MOODLE_API_LIVE") != "1" {
		t.Skip("set TEST_MOODLE_API_LIVE=1 for read-only Moodle validation")
	}
	client := newMoodleClient(os.Getenv("MOODLE_URL"), os.Getenv("MOODLE_USERNAME"), os.Getenv("MOODLE_PASSWORD"), os.Getenv("MOODLE_SERVICE"), &http.Client{Timeout: defaultHTTPTimeout})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	token, err := client.authenticate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	self, err := client.getSiteInfo(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	courses, err := client.getMyCourses(ctx, token, self.UserID)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, course := range courses {
		response, err := client.getAssignments(ctx, token, course.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range response.Courses {
			for _, task := range entry.Assignments {
				if task.TeamSubmission != 0 || task.NoSubmissions != 0 || task.DueDate <= time.Now().Unix() {
					continue
				}
				status, err := client.getOwnSubmissionStatus(ctx, token, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				if status.LastAttempt == nil || len(status.Warnings) > 0 {
					t.Fatal("own status did not include a reliable last attempt")
				}
				result := identifyPersonalReminder(course.ID, task, self.UserID, "fictional@example.test", status, client.baseURL)
				if !result.SubmissionDataOK {
					t.Fatalf("own status could not be classified: %s", result.UnavailableReason)
				}
				checked++
				if checked == 2 {
					t.Logf("authenticated and classified %d own upcoming assignments; no notifications published", checked)
					return
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no own upcoming assignment available for live status validation")
	}
}
