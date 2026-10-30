package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const reminderQueueName = "noodle.reminders"

const rabbitMQConnectionTimeout = 30 * time.Second

type reminderMessage struct {
	ID             string    `json:"id"`
	CourseID       int64     `json:"course_id"`
	AssignmentID   int64     `json:"assignment_id"`
	StudentID      int64     `json:"student_id"`
	Email          string    `json:"email"`
	AssignmentName string    `json:"assignment_name"`
	DueAt          time.Time `json:"due_at"`
}

type rabbitMQPublisher struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

func newReminderMessage(courseID, assignmentID, studentID int64, email, assignmentName string, dueAt time.Time) reminderMessage {
	return reminderMessage{
		ID:             reminderMessageID(courseID, assignmentID, studentID),
		CourseID:       courseID,
		AssignmentID:   assignmentID,
		StudentID:      studentID,
		Email:          email,
		AssignmentName: assignmentName,
		DueAt:          dueAt.UTC(),
	}
}

func reminderMessageID(courseID, assignmentID, studentID int64) string {
	return fmt.Sprintf("%d:%d:%d", courseID, assignmentID, studentID)
}

func marshalReminderMessage(message reminderMessage) ([]byte, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("encode reminder message: %w", err)
	}
	return body, nil
}

func newRabbitMQPublisher(ctx context.Context) (*rabbitMQPublisher, error) {
	rabbitMQURL := strings.TrimSpace(os.Getenv("RABBITMQ_URL"))
	if rabbitMQURL == "" {
		return nil, fmt.Errorf("RABBITMQ_URL is required")
	}

	connection, err := dialRabbitMQ(ctx, rabbitMQURL)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("connect to RabbitMQ: %w", ctxErr)
		}
		return nil, fmt.Errorf("connect to RabbitMQ: %w", sanitizeRabbitMQConnectionError(err, rabbitMQURL))
	}

	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}

	if _, err := channel.QueueDeclare(reminderQueueName, true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("declare RabbitMQ queue %q: %w", reminderQueueName, err)
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("enable RabbitMQ publisher confirms: %w", err)
	}

	return &rabbitMQPublisher{connection: connection, channel: channel}, nil
}

func dialRabbitMQ(ctx context.Context, rabbitMQURL string) (*amqp.Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dialFinished := make(chan struct{})
	connection, err := amqp.DialConfig(rabbitMQURL, amqp.Config{
		Dial: func(network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: rabbitMQConnectionTimeout}
			connection, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}

			deadline := time.Now().Add(rabbitMQConnectionTimeout)
			if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
				deadline = contextDeadline
			}
			if err := connection.SetDeadline(deadline); err != nil {
				_ = connection.Close()
				return nil, err
			}

			go func() {
				select {
				case <-ctx.Done():
					_ = connection.Close()
				case <-dialFinished:
				}
			}()
			return connection, nil
		},
	})
	close(dialFinished)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return connection, nil
}

func (publisher *rabbitMQPublisher) Publish(ctx context.Context, message reminderMessage) error {
	body, err := marshalReminderMessage(message)
	if err != nil {
		return err
	}

	confirmation, err := publisher.channel.PublishWithDeferredConfirmWithContext(
		ctx,
		"",
		reminderQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    message.ID,
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish reminder message: %w", err)
	}
	if confirmation == nil {
		return fmt.Errorf("publish reminder message: publisher confirmation unavailable")
	}

	acknowledged, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for RabbitMQ publisher confirmation: %w", err)
	}
	if !acknowledged {
		return fmt.Errorf("RabbitMQ rejected reminder message")
	}
	return nil
}

func (publisher *rabbitMQPublisher) Close() error {
	return errors.Join(publisher.channel.Close(), publisher.connection.Close())
}

func sanitizeRabbitMQConnectionError(err error, rabbitMQURL string) error {
	secrets := []string{rabbitMQURL}
	parsedURL, parseErr := url.Parse(rabbitMQURL)
	if parseErr == nil && parsedURL.User != nil {
		secrets = append(secrets, parsedURL.User.Username())
		if password, present := parsedURL.User.Password(); present {
			secrets = append(secrets, password)
		}
	}

	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return errors.New(message)
}
