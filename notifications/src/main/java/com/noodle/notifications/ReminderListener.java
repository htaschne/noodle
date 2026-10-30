package com.noodle.notifications;

import com.rabbitmq.client.Channel;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.amqp.core.Message;
import org.springframework.amqp.rabbit.annotation.RabbitListener;
import org.springframework.stereotype.Component;

import java.io.IOException;

@Component
class ReminderListener {
    private static final Logger logger = LoggerFactory.getLogger(ReminderListener.class);

    private final ReminderMessageParser parser;
    private final ReminderProcessor processor;

    ReminderListener(ReminderMessageParser parser, ReminderProcessor processor) {
        this.parser = parser;
        this.processor = processor;
    }

    @RabbitListener(queues = NotificationsApplication.REMINDER_QUEUE, ackMode = "MANUAL")
    void receive(Message message, Channel channel) throws IOException {
        long deliveryTag = message.getMessageProperties().getDeliveryTag();
        final ReminderMessage reminder;
        try {
            reminder = parser.parse(message.getBody());
        } catch (InvalidReminderMessageException exception) {
            logger.warn("invalid_reminder reason={}", exception.getMessage());
            channel.basicReject(deliveryTag, false);
            return;
        }

        try {
            processor.process(reminder);
            channel.basicAck(deliveryTag, false);
        } catch (NotificationProcessingException exception) {
            logger.error("notification_processing_failed reminder_id={} reason={}", reminder.id(), exception.getMessage());
            channel.basicReject(deliveryTag, false);
        }
    }
}
