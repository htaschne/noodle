# Notifications

Monitoring publishes persistent JSON to the durable `noodle.reminders` queue:

```json
{
  "id": "100:200:42",
  "course_id": 100,
  "assignment_id": 200,
  "student_id": 42,
  "email": "student@example.test",
  "assignment_name": "Example assignment",
  "assignment_url": "https://moodle.example.test/mod/assign/view.php?id=201",
  "due_at": "2026-11-20T15:00:00Z"
}
```

The link uses the assignment's **course module ID (`cmid`)**, not its assignment
ID. Monitoring skips assignments without a valid `cmid`. Notifications requires
an HTTP(S) URL without embedded credentials; older messages without
`assignment_url` are rejected. Deploy both services together.

The email contains the assignment name, deadline in UTC, and access link.
Successful delivery IDs (`course:assignment:student`) persist in SQLite before
RabbitMQ acknowledgment. Reprocessing a recorded ID does not send another email.

SMTP failures are logged and attempted up to three times, 500 ms apart. After
exhaustion, the message is rejected without requeue. Persistence failures also
reject the message. There is no dead-letter queue or automatic recovery of
rejected messages. A crash between SMTP acceptance and recording the ID can
produce a duplicate on redelivery.
