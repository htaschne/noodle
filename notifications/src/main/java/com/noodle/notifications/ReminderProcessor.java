package com.noodle.notifications;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.mail.MailException;
import org.springframework.stereotype.Service;

import java.time.Duration;

@Service
class ReminderProcessor {
    enum Result {
        DELIVERED,
        DUPLICATE
    }

    private static final Logger logger = LoggerFactory.getLogger(ReminderProcessor.class);

    private final NotificationHistoryRepository history;
    private final ReminderEmailService emailService;
    private final int maxAttempts;
    private final Duration retryDelay;

    ReminderProcessor(
            NotificationHistoryRepository history,
            ReminderEmailService emailService,
            @Value("${notifications.delivery.max-attempts}") int maxAttempts,
            @Value("${notifications.delivery.retry-delay}") Duration retryDelay
    ) {
        if (maxAttempts < 1) {
            throw new IllegalArgumentException("maxAttempts must be positive");
        }
        this.history = history;
        this.emailService = emailService;
        this.maxAttempts = maxAttempts;
        this.retryDelay = retryDelay;
    }

    Result process(ReminderMessage reminder) {
        if (history.wasDelivered(reminder.id())) {
            logger.warn("duplicate_reminder reminder_id={}", reminder.id());
            return Result.DUPLICATE;
        }

        sendWithRetry(reminder);
        if (!history.recordDelivered(reminder.id())) {
            logger.warn("notification_history_already_recorded reminder_id={}", reminder.id());
        }
        logger.info("notification_delivered reminder_id={}", reminder.id());
        return Result.DELIVERED;
    }

    private void sendWithRetry(ReminderMessage reminder) {
        MailException lastFailure = null;
        for (int attempt = 1; attempt <= maxAttempts; attempt++) {
            try {
                emailService.send(reminder);
                return;
            } catch (MailException exception) {
                lastFailure = exception;
                logger.warn(
                        "notification_delivery_retry reminder_id={} attempt={} max_attempts={}",
                        reminder.id(),
                        attempt,
                        maxAttempts
                );
                if (attempt < maxAttempts) {
                    pauseBeforeRetry();
                }
            }
        }
        throw new NotificationProcessingException("SMTP delivery failed after bounded retries", lastFailure);
    }

    private void pauseBeforeRetry() {
        try {
            Thread.sleep(retryDelay);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            throw new NotificationProcessingException("SMTP retry interrupted", exception);
        }
    }
}
