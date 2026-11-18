# Tests

Run the unit tests from the repository root:

```sh
(cd monitoring && go test ./...)
(cd notifications && mvn test)
```

| Service | Coverage |
| --- | --- |
| Monitoring | Deadline window, roles, submission states, individual confirmation of missing attempts, permission failures and message serialization |
| Notifications | Payload validation, email content, duplicate suppression, persisted history, bounded SMTP retries and acknowledgment |

## Recorded run: 2026-11-18

`go test ./...` passed. `mvn test` passed all 19 tests, with no failures or skips.
These results do not validate permissions or responses on the partner's Moodle
installation; see [Moodle setup](moodle.md).

## Live Moodle run: 2026-11-18

Started the stack with `docker compose up --build -d`, using the existing local
configuration and Mailpit. All four services started; RabbitMQ and Mailpit were
healthy. Both service images built, Go tests passed and all 19 Java tests passed.

The first monitoring cycle authenticated, discovered 20 courses and read 45
assignments. One assignment was within the deadline window, but its bulk
submission query returned `No access rights in module context`. The cycle reported
partial coverage, zero candidates and zero published reminders. The queue had one
consumer and no messages; Mailpit contained no emails.

Read-only checks confirmed that `mod_assign_get_submission_status` works for the
authenticated account but returns `nopermission` for another student. The roster
contained 177 students, with email exposed for only one. The current account cannot
support class-wide reminders. Repeat with an account allowed to read other
students' submissions and email addresses before considering delivery validated.

## Expanded read-only scan: 2026-11-18

Checked all 20 courses: 16 exposed assignments, totaling 45 visible tasks. All
45 bulk submission queries returned access warnings; none were accessible.
There were no request failures. Expanding the future deadline window from 24h to
14 days found three eligible individual tasks across two courses, versus one
task within 24h. All three remained blocked for class-wide submission queries.
Both rosters exposed only the authenticated student's email. Individual status
queries for that account succeeded for all three tasks (one `new`, two without
attempts). The scan published no reminders; the application's window remains 24h.
