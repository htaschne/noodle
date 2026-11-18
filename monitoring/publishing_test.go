package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type recordingReminderPublisher struct {
	messages     []reminderMessage
	err          error
	closed       bool
	afterPublish func()
}

func (publisher *recordingReminderPublisher) Publish(_ context.Context, message reminderMessage) error {
	if publisher.err != nil {
		return publisher.err
	}
	publisher.messages = append(publisher.messages, message)
	if publisher.afterPublish != nil {
		publisher.afterPublish()
	}
	return nil
}

func (publisher *recordingReminderPublisher) Close() error {
	publisher.closed = true
	return nil
}

type recordingPublisherFactory struct {
	calls     int
	publisher *recordingReminderPublisher
	err       error
}

func (factory *recordingPublisherFactory) New(context.Context) (reminderPublisher, error) {
	factory.calls++
	if factory.err != nil {
		return nil, factory.err
	}
	return factory.publisher, nil
}

func TestZeroCandidatesDoNotConnectToRabbitMQ(t *testing.T) {
	factory := &recordingPublisherFactory{publisher: &recordingReminderPublisher{}}

	published, err := publishReminderCandidates(context.Background(), factory.New, nil, discardLogger())
	if err != nil {
		t.Fatalf("publish zero candidates: %v", err)
	}
	if published != 0 || factory.calls != 0 {
		t.Fatalf("published=%d connections=%d, want 0 and 0", published, factory.calls)
	}
}

func TestValidCandidatesArePublished(t *testing.T) {
	publisher := &recordingReminderPublisher{}
	factory := &recordingPublisherFactory{publisher: publisher}
	candidates := []reminderCandidate{{
		CourseID:      95406,
		AssignmentID:  123456,
		UserID:        42,
		Email:         "student@example.test",
		Assignment:    "Trabalho 2",
		AssignmentURL: "https://moodle.example.test/mod/assign/view.php?id=654321",
		DueDate:       time.Date(2026, time.October, 31, 15, 0, 0, 0, time.UTC),
	}}

	published, err := publishReminderCandidates(context.Background(), factory.New, candidates, discardLogger())
	if err != nil {
		t.Fatalf("publish candidates: %v", err)
	}
	if published != 1 || factory.calls != 1 || len(publisher.messages) != 1 {
		t.Fatalf("published=%d connections=%d messages=%d, want 1, 1, 1", published, factory.calls, len(publisher.messages))
	}
	if publisher.messages[0].ID != "95406:123456:42" {
		t.Fatalf("message ID = %q", publisher.messages[0].ID)
	}
	if !publisher.closed {
		t.Fatal("publisher was not closed after publication")
	}
}

func TestAmbiguousIdentificationProducesNoConnection(t *testing.T) {
	user := enrolledUser{
		ID:    3,
		Email: "student@example.test",
		Roles: []moodleRole{{ShortName: "student"}, {ShortName: "custom-role"}},
	}
	assignment := assignment{CMID: 201, ID: 2, DueDate: time.Now().Add(time.Hour).Unix()}
	response := submissionsResponse{Assignments: []assignmentSubmissions{{
		AssignmentID: assignment.ID,
		Submissions:  []submission{{UserID: user.ID, Status: "draft", Latest: 1}},
	}}}
	identification := identifyReminderCandidates(1, assignment, []enrolledUser{user}, response, "https://moodle.example.test")
	factory := &recordingPublisherFactory{publisher: &recordingReminderPublisher{}}

	published, err := publishReminderCandidates(context.Background(), factory.New, identification.Candidates, discardLogger())
	if err != nil {
		t.Fatalf("publish ambiguous candidates: %v", err)
	}
	if published != 0 || factory.calls != 0 {
		t.Fatalf("ambiguous data published=%d connections=%d, want 0 and 0", published, factory.calls)
	}
}

func TestPublicationErrorsAreLoggedAndPropagated(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	publisher := &recordingReminderPublisher{err: errors.New("broker unavailable")}
	factory := &recordingPublisherFactory{publisher: publisher}
	candidates := []reminderCandidate{{
		CourseID:     95406,
		AssignmentID: 123456,
		UserID:       42,
		Email:        "student@example.test",
	}}

	published, err := publishReminderCandidates(context.Background(), factory.New, candidates, logger)
	if err == nil || !strings.Contains(err.Error(), "broker unavailable") {
		t.Fatalf("expected propagated publication error, got %v", err)
	}
	if published != 0 || !publisher.closed {
		t.Fatalf("published=%d closed=%t, want 0 and true", published, publisher.closed)
	}
	logs := output.String()
	if !strings.Contains(logs, "could not publish reminder candidate") || !strings.Contains(logs, "level=ERROR") {
		t.Fatalf("publication error was not logged: %q", logs)
	}
	if strings.Contains(logs, "student@example.test") {
		t.Fatalf("publication log exposed student email: %q", logs)
	}
}

func TestContextCancellationStopsBeforeConnecting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	factory := &recordingPublisherFactory{publisher: &recordingReminderPublisher{}}
	candidates := []reminderCandidate{{CourseID: 1, AssignmentID: 2, UserID: 3}}

	published, err := publishReminderCandidates(ctx, factory.New, candidates, discardLogger())
	if !errors.Is(err, context.Canceled) || published != 0 || factory.calls != 0 {
		t.Fatalf("cancelled publish: published=%d connections=%d err=%v", published, factory.calls, err)
	}
}

func TestContextCancellationStopsPublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	publisher := &recordingReminderPublisher{afterPublish: cancel}
	factory := &recordingPublisherFactory{publisher: publisher}
	candidates := []reminderCandidate{
		{CourseID: 1, AssignmentID: 2, UserID: 3},
		{CourseID: 4, AssignmentID: 5, UserID: 6},
	}

	published, err := publishReminderCandidates(ctx, factory.New, candidates, discardLogger())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publish error = %v, want context.Canceled", err)
	}
	if published != 1 || len(publisher.messages) != 1 {
		t.Fatalf("published=%d messages=%d, want 1 and 1", published, len(publisher.messages))
	}
	if !publisher.closed {
		t.Fatal("publisher was not closed after cancellation")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
