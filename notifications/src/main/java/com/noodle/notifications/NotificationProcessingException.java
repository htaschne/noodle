package com.noodle.notifications;

final class NotificationProcessingException extends RuntimeException {
    NotificationProcessingException(String message) {
        super(message);
    }

    NotificationProcessingException(String message, Throwable cause) {
        super(message, cause);
    }
}
