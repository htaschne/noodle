//go:build integration

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRabbitMQPublisherIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv("RABBITMQ_URL")) == "" {
		t.Fatal("RABBITMQ_URL is required for the RabbitMQ integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	publisher, err := newRabbitMQPublisher(ctx)
	if err != nil {
		t.Fatalf("create RabbitMQ publisher: %v", err)
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			t.Errorf("close RabbitMQ publisher: %v", err)
		}
	}()

	before, err := publisher.channel.QueueInspect(reminderQueueName)
	if err != nil {
		t.Fatalf("inspect reminder queue before publish: %v", err)
	}

	message := newReminderMessage(
		95406,
		123456,
		42,
		"student@example.test",
		"Trabalho 2",
		"https://moodle.example.test/mod/assign/view.php?id=654321",
		time.Date(2026, time.October, 31, 15, 0, 0, 0, time.UTC),
	)
	if err := publisher.Publish(ctx, message); err != nil {
		t.Fatalf("publish fictional reminder: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		after, err := publisher.channel.QueueInspect(reminderQueueName)
		if err != nil {
			t.Fatalf("inspect reminder queue after publish: %v", err)
		}
		if after.Messages >= before.Messages+1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("RabbitMQ queue depth did not increase after confirmed publish")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
