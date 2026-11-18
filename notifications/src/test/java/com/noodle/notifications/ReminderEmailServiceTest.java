package com.noodle.notifications;

import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.mail.SimpleMailMessage;
import org.springframework.mail.javamail.JavaMailSender;

import java.time.Instant;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;

class ReminderEmailServiceTest {
    @Test
    void formatsPlainTextReminder() {
        JavaMailSender mailSender = mock(JavaMailSender.class);
        ReminderEmailService service = new ReminderEmailService(mailSender, "noodle@example.test");

        service.send(TestFixtures.reminder());

        ArgumentCaptor<SimpleMailMessage> captor = ArgumentCaptor.forClass(SimpleMailMessage.class);
        verify(mailSender).send(captor.capture());
        SimpleMailMessage email = captor.getValue();
        assertEquals("noodle@example.test", email.getFrom());
        assertEquals("student@example.test", email.getTo()[0]);
        assertEquals("Moodle assignment reminder: Trabalho 2", email.getSubject());
        assertTrue(email.getText().contains("Assignment: Trabalho 2"));
        assertTrue(email.getText().contains("Due: " + Instant.parse("2026-10-31T15:00:00Z")));
        assertTrue(email.getText().contains("Link: https://moodle.example.test/mod/assign/view.php?id=654321"));
        assertTrue(email.getText().contains("submit this assignment through Moodle"));
    }
}
