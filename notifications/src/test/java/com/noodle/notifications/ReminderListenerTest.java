package com.noodle.notifications;

import com.rabbitmq.client.Channel;
import org.junit.jupiter.api.Test;
import org.springframework.amqp.core.Message;
import org.springframework.amqp.core.MessageProperties;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class ReminderListenerTest {
    private static final long DELIVERY_TAG = 17L;

    @Test
    void acknowledgesSuccessfulDelivery() throws Exception {
        ReminderMessageParser parser = mock(ReminderMessageParser.class);
        ReminderProcessor processor = mock(ReminderProcessor.class);
        Channel channel = mock(Channel.class);
        ReminderMessage reminder = TestFixtures.reminder();
        when(parser.parse(any())).thenReturn(reminder);

        new ReminderListener(parser, processor).receive(message(), channel);

        verify(processor).process(reminder);
        verify(channel).basicAck(DELIVERY_TAG, false);
        verify(channel, never()).basicReject(DELIVERY_TAG, false);
    }

    @Test
    void acknowledgesDuplicateWithoutResending() throws Exception {
        ReminderMessageParser parser = mock(ReminderMessageParser.class);
        ReminderProcessor processor = mock(ReminderProcessor.class);
        Channel channel = mock(Channel.class);
        ReminderMessage reminder = TestFixtures.reminder();
        when(parser.parse(any())).thenReturn(reminder);
        when(processor.process(reminder)).thenReturn(ReminderProcessor.Result.DUPLICATE);

        new ReminderListener(parser, processor).receive(message(), channel);

        verify(channel).basicAck(DELIVERY_TAG, false);
        verify(channel, never()).basicReject(DELIVERY_TAG, false);
    }

    @Test
    void rejectsInvalidMessageWithoutRequeue() throws Exception {
        ReminderMessageParser parser = mock(ReminderMessageParser.class);
        ReminderProcessor processor = mock(ReminderProcessor.class);
        Channel channel = mock(Channel.class);
        when(parser.parse(any())).thenThrow(new InvalidReminderMessageException("invalid"));

        new ReminderListener(parser, processor).receive(message(), channel);

        verify(channel).basicReject(DELIVERY_TAG, false);
        verify(channel, never()).basicAck(DELIVERY_TAG, false);
        verify(processor, never()).process(any());
    }

    @Test
    void rejectsFailedDeliveryWithoutAcknowledging() throws Exception {
        ReminderMessageParser parser = mock(ReminderMessageParser.class);
        ReminderProcessor processor = mock(ReminderProcessor.class);
        Channel channel = mock(Channel.class);
        ReminderMessage reminder = TestFixtures.reminder();
        when(parser.parse(any())).thenReturn(reminder);
        doThrow(new NotificationProcessingException("delivery failed")).when(processor).process(reminder);

        new ReminderListener(parser, processor).receive(message(), channel);

        verify(channel).basicReject(DELIVERY_TAG, false);
        verify(channel, never()).basicAck(DELIVERY_TAG, false);
    }

    private Message message() {
        MessageProperties properties = new MessageProperties();
        properties.setDeliveryTag(DELIVERY_TAG);
        return new Message("{}".getBytes(), properties);
    }
}
