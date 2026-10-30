package com.noodle.notifications;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Path;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class NotificationHistoryRepositoryTest {
    @TempDir
    Path tempDirectory;

    @Test
    void persistsUniqueDeliveryAcrossRepositoryRestarts() {
        Path database = tempDirectory.resolve("notifications.db");
        NotificationHistoryRepository first = new NotificationHistoryRepository(database.toString());
        first.initialize();

        assertFalse(first.wasDelivered("1:2:3"));
        assertTrue(first.recordDelivered("1:2:3"));
        assertFalse(first.recordDelivered("1:2:3"));

        NotificationHistoryRepository restarted = new NotificationHistoryRepository(database.toString());
        restarted.initialize();
        assertTrue(restarted.wasDelivered("1:2:3"));
    }
}
