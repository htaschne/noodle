package com.noodle.notifications;

import jakarta.annotation.PostConstruct;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Repository;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.time.Instant;

@Repository
class NotificationHistoryRepository {
    private final Path databasePath;
    private final String databaseUrl;

    NotificationHistoryRepository(@Value("${notifications.database-path}") String databasePath) {
        this.databasePath = Path.of(databasePath).toAbsolutePath().normalize();
        this.databaseUrl = "jdbc:sqlite:" + this.databasePath;
    }

    @PostConstruct
    void initialize() {
        Path parent = databasePath.getParent();
        try {
            if (parent != null) {
                Files.createDirectories(parent);
            }
        } catch (IOException exception) {
            throw new NotificationProcessingException("could not create notification database directory", exception);
        }

        try (Connection connection = connect();
             PreparedStatement statement = connection.prepareStatement("""
                     CREATE TABLE IF NOT EXISTS delivered_notifications (
                         message_id TEXT PRIMARY KEY,
                         delivered_at TEXT NOT NULL
                     )
                     """)) {
            statement.execute();
        } catch (SQLException exception) {
            throw new NotificationProcessingException("could not initialize notification history", exception);
        }
    }

    boolean wasDelivered(String messageId) {
        try (Connection connection = connect();
             PreparedStatement statement = connection.prepareStatement(
                     "SELECT 1 FROM delivered_notifications WHERE message_id = ?")) {
            statement.setString(1, messageId);
            try (ResultSet results = statement.executeQuery()) {
                return results.next();
            }
        } catch (SQLException exception) {
            throw new NotificationProcessingException("could not read notification history", exception);
        }
    }

    boolean recordDelivered(String messageId) {
        try (Connection connection = connect();
             PreparedStatement statement = connection.prepareStatement("""
                     INSERT INTO delivered_notifications (message_id, delivered_at)
                     VALUES (?, ?)
                     ON CONFLICT(message_id) DO NOTHING
                     """)) {
            statement.setString(1, messageId);
            statement.setString(2, Instant.now().toString());
            return statement.executeUpdate() == 1;
        } catch (SQLException exception) {
            throw new NotificationProcessingException("could not record notification delivery", exception);
        }
    }

    private Connection connect() throws SQLException {
        return DriverManager.getConnection(databaseUrl);
    }
}
