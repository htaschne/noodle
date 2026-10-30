package com.noodle.notifications;

final class InvalidReminderMessageException extends Exception {
    InvalidReminderMessageException(String message) {
        super(message);
    }

    InvalidReminderMessageException(String message, Throwable cause) {
        super(message, cause);
    }
}
