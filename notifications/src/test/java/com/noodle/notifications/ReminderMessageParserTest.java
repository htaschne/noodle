package com.noodle.notifications;

import org.junit.jupiter.api.Test;
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
                  "due_at": "2026-10-31T15:00:00Z"
                }
                """.getBytes(StandardCharsets.UTF_8));

        assertEquals("95406:123456:42", message.id());
        assertEquals(95406, message.courseId());
        assertEquals(123456, message.assignmentId());
        assertEquals(42, message.studentId());
        assertEquals("student@example.test", message.email());
        assertEquals("Trabalho 2", message.assignmentName());
        assertEquals(Instant.parse("2026-10-31T15:00:00Z"), message.dueAt());
    }

    @Test
    void rejectsMalformedJson() {
        assertThrows(
                InvalidReminderMessageException.class,
                () -> parser.parse("not-json".getBytes(StandardCharsets.UTF_8))
        );
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
                          "due_at": "2026-10-31T15:00:00Z"
                        }
                        """.getBytes(StandardCharsets.UTF_8))
        );
    }
}
