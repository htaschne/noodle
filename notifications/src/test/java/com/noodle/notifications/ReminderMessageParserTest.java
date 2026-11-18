package com.noodle.notifications;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.NullAndEmptySource;
import org.junit.jupiter.params.provider.ValueSource;
import tools.jackson.databind.json.JsonMapper;

import java.nio.charset.StandardCharsets;
import java.time.Instant;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class ReminderMessageParserTest {
    private final ReminderMessageParser parser = new ReminderMessageParser(
            JsonMapper.builder().build()
    );

    @Test
    void deserializesMonitoringContract() throws Exception {
        ReminderMessage message = parser.parse("""
                {
                  "id": "95406:123456:42",
                  "course_id": 95406,
                  "assignment_id": 123456,
                  "student_id": 42,
                  "email": "student@example.test",
                  "assignment_name": "Trabalho 2",
                  "assignment_url": "https://moodle.example.test/mod/assign/view.php?id=654321",
                  "due_at": "2026-10-31T15:00:00Z"
                }
                """.getBytes(StandardCharsets.UTF_8));

        assertEquals("95406:123456:42", message.id());
        assertEquals(95406, message.courseId());
        assertEquals(123456, message.assignmentId());
        assertEquals(42, message.studentId());
        assertEquals("student@example.test", message.email());
        assertEquals("Trabalho 2", message.assignmentName());
        assertEquals("https://moodle.example.test/mod/assign/view.php?id=654321", message.assignmentUrl());
        assertEquals(Instant.parse("2026-10-31T15:00:00Z"), message.dueAt());
    }

    @Test
    void rejectsMalformedJson() {
        assertThrows(
                InvalidReminderMessageException.class,
                () -> parser.parse("not-json".getBytes(StandardCharsets.UTF_8))
        );
    }

    @ParameterizedTest
    @NullAndEmptySource
    @ValueSource(strings = {"javascript:alert(1)", "/mod/assign/view.php?id=1",
            "https://user:secret@moodle.example.test/task", "https://", "https://moodle.example.test/bad link"})
    void rejectsMissingOrInvalidAssignmentUrl(String url) throws Exception {
        ReminderMessage valid = TestFixtures.reminder();
        ReminderMessage invalid = new ReminderMessage(valid.id(), valid.courseId(), valid.assignmentId(),
                valid.studentId(), valid.email(), valid.assignmentName(), url, valid.dueAt());
        byte[] payload = JsonMapper.builder().build().writeValueAsBytes(invalid);

        assertThrows(InvalidReminderMessageException.class, () -> parser.parse(payload));
    }

    @Test
    void rejectsMismatchedDeterministicId() {
        assertThrows(
                InvalidReminderMessageException.class,
                () -> parser.parse("""
                        {
                          "id": "wrong",
                          "course_id": 1,
                          "assignment_id": 2,
                          "student_id": 3,
                          "email": "student@example.test",
                          "assignment_name": "Assignment",
                          "assignment_url": "https://moodle.example.test/mod/assign/view.php?id=654321",
                          "due_at": "2026-10-31T15:00:00Z"
                        }
                        """.getBytes(StandardCharsets.UTF_8))
        );
    }
}
