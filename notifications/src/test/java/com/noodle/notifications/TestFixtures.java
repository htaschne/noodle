package com.noodle.notifications;

import java.time.Instant;

final class TestFixtures {
    private TestFixtures() {
    }

    static ReminderMessage reminder() {
        return new ReminderMessage(
                "95406:123456:42",
                95406,
                123456,
                42,
                "student@example.test",
                "Trabalho 2",
                Instant.parse("2026-10-31T15:00:00Z")
        );
    }
}
