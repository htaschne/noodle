package com.noodle.notifications;

import org.springframework.stereotype.Component;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.json.JsonMapper;

import java.nio.charset.StandardCharsets;
import java.net.URI;
import java.net.URISyntaxException;
import java.util.regex.Pattern;

@Component
final class ReminderMessageParser {
    private static final Pattern EMAIL = Pattern.compile("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$");

    private final JsonMapper jsonMapper;

    ReminderMessageParser(JsonMapper jsonMapper) {
        this.jsonMapper = jsonMapper;
    }

    ReminderMessage parse(byte[] payload) throws InvalidReminderMessageException {
        final ReminderMessage message;
        try {
            message = jsonMapper.readValue(new String(payload, StandardCharsets.UTF_8), ReminderMessage.class);
        } catch (JacksonException exception) {
            throw new InvalidReminderMessageException("message is not valid reminder JSON", exception);
        }

        if (message == null) {
            throw new InvalidReminderMessageException("message must not be null");
        }
        if (message.courseId() <= 0 || message.assignmentId() <= 0 || message.studentId() <= 0) {
            throw new InvalidReminderMessageException("message identifiers must be positive");
        }
        String expectedId = "%d:%d:%d".formatted(message.courseId(), message.assignmentId(), message.studentId());
        if (!expectedId.equals(message.id())) {
            throw new InvalidReminderMessageException("message ID does not match its identifiers");
        }
        if (message.email() == null || !EMAIL.matcher(message.email()).matches()) {
            throw new InvalidReminderMessageException("message email is invalid");
        }
        if (message.assignmentName() == null || message.assignmentName().isBlank()) {
            throw new InvalidReminderMessageException("assignment name is required");
        }
        try {
            URI url = new URI(message.assignmentUrl() == null ? "" : message.assignmentUrl());
            if (!("https".equalsIgnoreCase(url.getScheme()) || "http".equalsIgnoreCase(url.getScheme()))
                    || url.getHost() == null || url.getUserInfo() != null) {
                throw new InvalidReminderMessageException("assignment URL must be an HTTP or HTTPS URL without credentials");
            }
        } catch (URISyntaxException exception) {
            throw new InvalidReminderMessageException("assignment URL is invalid", exception);
        }
        if (message.dueAt() == null) {
            throw new InvalidReminderMessageException("due date is required");
        }
        return message;
    }
}
