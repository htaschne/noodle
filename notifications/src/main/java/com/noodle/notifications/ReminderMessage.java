package com.noodle.notifications;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.time.Instant;

public record ReminderMessage(
        @JsonProperty("id") String id,
        @JsonProperty("course_id") long courseId,
        @JsonProperty("assignment_id") long assignmentId,
        @JsonProperty("student_id") long studentId,
        @JsonProperty("email") String email,
        @JsonProperty("assignment_name") String assignmentName,
        @JsonProperty("due_at") Instant dueAt
) {
}
