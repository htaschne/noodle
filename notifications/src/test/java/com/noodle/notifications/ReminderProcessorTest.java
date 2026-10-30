package com.noodle.notifications;

import org.junit.jupiter.api.Test;
import org.springframework.mail.MailSendException;

import java.time.Duration;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class ReminderProcessorTest {
    @Test
    void deliversAndRecordsReminder() {
        NotificationHistoryRepository history = mock(NotificationHistoryRepository.class);
        ReminderEmailService emailService = mock(ReminderEmailService.class);
        ReminderMessage reminder = TestFixtures.reminder();
        when(history.recordDelivered(reminder.id())).thenReturn(true);
        ReminderProcessor processor = new ReminderProcessor(history, emailService, 3, Duration.ZERO);

        assertEquals(ReminderProcessor.Result.DELIVERED, processor.process(reminder));

        verify(emailService).send(reminder);
        verify(history).recordDelivered(reminder.id());
    }

    @Test
    void suppressesAlreadyDeliveredReminder() {
        NotificationHistoryRepository history = mock(NotificationHistoryRepository.class);
        ReminderEmailService emailService = mock(ReminderEmailService.class);
        ReminderMessage reminder = TestFixtures.reminder();
        when(history.wasDelivered(reminder.id())).thenReturn(true);
        ReminderProcessor processor = new ReminderProcessor(history, emailService, 3, Duration.ZERO);

        assertEquals(ReminderProcessor.Result.DUPLICATE, processor.process(reminder));

        verify(emailService, never()).send(reminder);
        verify(history, never()).recordDelivered(reminder.id());
    }

    @Test
    void retriesSmtpFailureOnlyUpToConfiguredLimit() {
        NotificationHistoryRepository history = mock(NotificationHistoryRepository.class);
        ReminderEmailService emailService = mock(ReminderEmailService.class);
        ReminderMessage reminder = TestFixtures.reminder();
        doThrow(new MailSendException("SMTP unavailable")).when(emailService).send(reminder);
        ReminderProcessor processor = new ReminderProcessor(history, emailService, 3, Duration.ZERO);

        assertThrows(NotificationProcessingException.class, () -> processor.process(reminder));

        verify(emailService, times(3)).send(reminder);
        verify(history, never()).recordDelivered(reminder.id());
    }
}
