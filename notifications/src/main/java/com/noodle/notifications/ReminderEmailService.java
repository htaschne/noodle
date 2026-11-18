package com.noodle.notifications;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.mail.SimpleMailMessage;
import org.springframework.mail.javamail.JavaMailSender;
import org.springframework.stereotype.Service;

@Service
class ReminderEmailService {
    private final JavaMailSender mailSender;
    private final String sender;

    ReminderEmailService(JavaMailSender mailSender, @Value("${notifications.mail.from}") String sender) {
        this.mailSender = mailSender;
        this.sender = sender;
    }

    void send(ReminderMessage reminder) {
        SimpleMailMessage email = new SimpleMailMessage();
        email.setFrom(sender);
        email.setTo(reminder.email());
        email.setSubject("Moodle assignment reminder: " + reminder.assignmentName());
        email.setText("""
                Assignment: %s
                Due: %s
                Link: %s

                Please submit this assignment through Moodle before the deadline.
                """.formatted(reminder.assignmentName(), reminder.dueAt(), reminder.assignmentUrl()));
        mailSender.send(email);
    }
}
