//go:build acceptance

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func acceptanceCompose(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	base := []string{"compose", "--env-file", os.DevNull, "-p", os.Getenv("ACCEPTANCE_COMPOSE_PROJECT"), "-f", os.Getenv("ACCEPTANCE_COMPOSE_FILE")}
	output, err := exec.CommandContext(ctx, "docker", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("test stack operation failed: %v\n%s", err, output)
	}
	return string(output)
}

func acceptanceWait(t *testing.T, description string, check func() bool) {
	t.Helper()
	until := time.Now().Add(30 * time.Second)
	for time.Now().Before(until) {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func acceptanceHTTP(t *testing.T, method, path string, body []byte) []byte {
	t.Helper()
	req, err := http.NewRequest(method, os.Getenv("ACCEPTANCE_MAILPIT_URL")+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("Mailpit API failed: status=%d error=%v", response.StatusCode, err)
	}
	return data
}

func acceptanceMailCount(t *testing.T) int {
	t.Helper()
	var result struct{ Total int }
	if err := json.Unmarshal(acceptanceHTTP(t, http.MethodGet, "/api/v1/messages", nil), &result); err != nil {
		t.Fatal(err)
	}
	return result.Total
}

func TestPersonalAcceptance(t *testing.T) {
	for _, key := range []string{"RABBITMQ_URL", "ACCEPTANCE_MAILPIT_URL", "ACCEPTANCE_COMPOSE_PROJECT", "ACCEPTANCE_COMPOSE_FILE"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s is required; run tests/acceptance/run.py", key)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	observer, err := newRabbitMQPublisher(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	acceptanceWait(t, "notification consumer", func() bool {
		q, e := observer.channel.QueueInspect(reminderQueueName)
		return e == nil && q.Consumers > 0
	})
	if count := acceptanceMailCount(t); count != 0 {
		t.Fatalf("test mailbox must be empty; got %d", count)
	}
	now := time.Now()
	assignments := []assignment{
		{ID: 101, CMID: 1001, Name: "Pending essay", DueDate: now.Add(time.Hour).Unix()},
		{ID: 102, CMID: 1002, Name: "Submitted essay", DueDate: now.Add(time.Hour).Unix()},
		{ID: 103, CMID: 1003, Name: "Future essay", DueDate: now.Add(48 * time.Hour).Unix()},
		{ID: 104, CMID: 1004, Name: "Overdue essay", DueDate: now.Add(-time.Hour).Unix()},
		{ID: 106, CMID: 1006, Name: "Group essay", DueDate: now.Add(time.Hour).Unix(), TeamSubmission: 1},
	}
	statuses := map[int64]submissionStatusResponse{
		101: pendingOwnStatus(t, ""),
		102: pendingOwnStatus(t, `,"submission":{"userid":42,"status":"submitted"}`),
	}
	server := personalMoodleServer(t, assignments, statuses, nil)
	defer server.Close()
	client := newMoodleClient(server.URL, "fictional-student", "fictional-password", "service", server.Client())
	client.email = "student@example.test"
	if err := monitorCycle(ctx, client, discardLogger()); err != nil {
		t.Fatal(err)
	}
	acceptanceWait(t, "one pending-work email", func() bool { return acceptanceMailCount(t) == 1 })
	body := string(acceptanceHTTP(t, http.MethodGet, "/view/latest.txt", nil))
	if !strings.Contains(body, "Pending essay") || !strings.Contains(body, "/mod/assign/view.php?id=1001") || !strings.Contains(body, time.Unix(assignments[0].DueDate, 0).UTC().Format(time.RFC3339)) {
		t.Fatalf("email lacks required content: %s", body)
	}
	t.Log("PASS criterion 1: never-started assignment due within 24h produced one Mailpit email containing name, deadline and link")
	acceptanceWait(t, "completed first delivery", func() bool {
		return strings.Contains(acceptanceCompose(t, "logs", "--no-color", "notifications"), "notification_delivered reminder_id=100:101:42")
	})
	if acceptanceMailCount(t) != 1 {
		t.Fatal("submitted, future, overdue or group work produced an email")
	}
	t.Log("PASS criterion 2: submitted, outside-window, overdue and group assignments produced no additional email")

	// Replay an actual completed message after restarting both the consumer and
	// the monitoring client; the SQLite history volume remains mounted.
	acceptanceCompose(t, "restart", "notifications")
	acceptanceWait(t, "restarted consumer", func() bool {
		q, e := observer.channel.QueueInspect(reminderQueueName)
		return e == nil && q.Consumers > 0
	})
	client = newMoodleClient(server.URL, "fictional-student", "fictional-password", "service", server.Client())
	client.email = "student@example.test"
	if err := monitorCycle(ctx, client, discardLogger()); err != nil {
		t.Fatal(err)
	}
	acceptanceWait(t, "persisted duplicate suppression", func() bool {
		return strings.Contains(acceptanceCompose(t, "logs", "--no-color", "notifications"), "duplicate_reminder reminder_id=100:101:42")
	})
	if acceptanceMailCount(t) != 1 {
		t.Fatal("replay after restart sent duplicate email")
	}
	t.Log("PASS criterion 3: replay after monitoring and notification restarts sent no duplicate email")

	// Make the next pending task newly eligible, then reject SMTP until the
	// consumer records a retry. Disable Chaos before the next bounded attempt.
	acceptanceHTTP(t, http.MethodPut, "/api/v1/chaos", []byte(`{"Sender":{"ErrorCode":451,"Probability":100}}`))
	defer acceptanceHTTP(t, http.MethodPut, "/api/v1/chaos", []byte(`{}`))
	retryTask := assignment{ID: 105, CMID: 1005, Name: "Draft retry essay", DueDate: now.Add(time.Hour).Unix()}
	retryServer := personalMoodleServer(t, []assignment{retryTask}, map[int64]submissionStatusResponse{105: pendingOwnStatus(t, `,"submission":{"userid":42,"status":"draft"}`)}, nil)
	defer retryServer.Close()
	retryClient := newMoodleClient(retryServer.URL, "fictional-student", "fictional-password", "service", retryServer.Client())
	retryClient.email = "student@example.test"
	if err := monitorCycle(ctx, retryClient, discardLogger()); err != nil {
		t.Fatal(err)
	}
	acceptanceWait(t, "recorded SMTP failure and retry", func() bool {
		return strings.Contains(acceptanceCompose(t, "logs", "--no-color", "notifications"), "notification_delivery_retry reminder_id=100:105:42 attempt=1")
	})
	acceptanceHTTP(t, http.MethodPut, "/api/v1/chaos", []byte(`{}`))
	acceptanceWait(t, "successful SMTP retry", func() bool { return acceptanceMailCount(t) == 2 })
	acceptanceWait(t, "retry delivery recorded", func() bool {
		return strings.Contains(acceptanceCompose(t, "logs", "--no-color", "notifications"), "notification_delivered reminder_id=100:105:42")
	})
	t.Log("PASS criterion 4: Mailpit SMTP 451 failure was logged; retry recovered and delivered the draft reminder")
	t.Log(fmt.Sprintf("Acceptance result: %d captured emails, fictional accounts only, no live Moodle requests", acceptanceMailCount(t)))
}
