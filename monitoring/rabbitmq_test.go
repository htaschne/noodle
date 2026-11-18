package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRabbitMQPublisherHonorsCancelledContext(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqp://example-user:example-password@192.0.2.1:5672/")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newRabbitMQPublisher(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("publisher error = %v, want context.Canceled", err)
	}
}

func TestReminderMessageIDIsDeterministic(t *testing.T) {
	const expected = "95406:123456:42"
	if actual := reminderMessageID(95406, 123456, 42); actual != expected {
		t.Fatalf("message ID = %q, want %q", actual, expected)
	}
	if actual := reminderMessageID(95406, 123456, 42); actual != expected {
		t.Fatalf("repeated message ID = %q, want %q", actual, expected)
	}
}

func TestRabbitMQConnectionErrorsAreRedacted(t *testing.T) {
	const rabbitMQURL = "amqp://example-user:example-password@localhost:5672/"
	err := sanitizeRabbitMQConnectionError(
		errors.New("dial "+rabbitMQURL+" for example-user with example-password"),
		rabbitMQURL,
	)

	for _, secret := range []string{rabbitMQURL, "example-user", "example-password"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("connection error exposed %q", secret)
		}
	}
}

func TestReminderMessageSerialization(t *testing.T) {
	dueAt := time.Date(2026, time.October, 31, 12, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	message := newReminderMessage(95406, 123456, 42, "student@example.test", "Trabalho 2", "https://moodle.example.test/mod/assign/view.php?id=654321", dueAt)

	body, err := marshalReminderMessage(message)
	if err != nil {
		t.Fatalf("marshal reminder message: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode reminder JSON: %v", err)
	}
	if len(decoded) != 8 {
		t.Fatalf("JSON field count = %d, want 8: %s", len(decoded), body)
	}

	expected := map[string]any{
		"id":              "95406:123456:42",
		"course_id":       float64(95406),
		"assignment_id":   float64(123456),
		"student_id":      float64(42),
		"email":           "student@example.test",
		"assignment_name": "Trabalho 2",
		"assignment_url":  "https://moodle.example.test/mod/assign/view.php?id=654321",
		"due_at":          "2026-10-31T15:00:00Z",
	}
	for field, want := range expected {
		if got := decoded[field]; got != want {
			t.Fatalf("field %q = %#v, want %#v", field, got, want)
		}
	}
}
