package com.noodle.notifications;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.core.Queue;
import org.springframework.amqp.core.QueueBuilder;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.annotation.Bean;
import org.springframework.context.event.EventListener;

@SpringBootApplication
public class NotificationsApplication {
    static final String REMINDER_QUEUE = "noodle.reminders";

    private static final Logger logger = LoggerFactory.getLogger(NotificationsApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(NotificationsApplication.class, args);
    }

    @Bean
    Queue reminderQueue() {
        return QueueBuilder.durable(REMINDER_QUEUE).build();
    }

    @EventListener(ApplicationReadyEvent.class)
    void logStartup() {
        logger.info("notifications_service_started queue={}", REMINDER_QUEUE);
    }
}
