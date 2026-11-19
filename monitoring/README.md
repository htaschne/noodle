# Moodle Monitoring Service

The monitoring service is a long-running worker that uses the personal Moodle API with `MOODLE_SOURCE=api`, or an iCalendar feed with `MOODLE_SOURCE=ical`. In `auto` mode (the default), a configured calendar URL takes precedence over the API. Both sources publish reminders through the existing RabbitMQ pipeline for deadlines in `(now, now + 24h]`. The cycle repeats after `MONITOR_INTERVAL` (default `1h`). It does not send email or modify Moodle data.

## Calendar configuration

Follow the export steps in [`.env.example`](../.env.example), then set these values in the ignored `.env` file:

```dotenv
MOODLE_SOURCE=ical
MOODLE_ICAL_URL=<calendar export URL>
MOODLE_ICAL_EMAIL=<feed owner's reminder email>
MOODLE_ICAL_TIMEZONE=America/Sao_Paulo
```

The export URL contains an authentication token. Keep it secret and never commit it. Calendar mode does not require `MOODLE_URL`, API credentials, or privileged web-service access. Existing API configuration can stay in place; set `MOODLE_SOURCE=api` to select personal API mode while preserving the calendar URL. `MOODLE_ICAL_EMAIL` falls back to `NOTIFICATION_EMAIL`; API mode accepts the older `MOODLE_ICAL_EMAIL` as a recipient fallback. A failed calendar request is retried on the next scheduled cycle, without switching sources.

`TZID` values use the embedded IANA timezone database. Floating times and all-day dates use the feed's `X-WR-TIMEZONE`, then `MOODLE_ICAL_TIMEZONE`, then UTC. An all-day deadline is the last second before the exclusive `DTEND` date, or the end of its single day when no end is given. For timed deadline events, `DTEND` (or `DURATION`) is used when supplied; otherwise `DTSTART` is the deadline.

Only explicitly labelled deadlines are included: English labels such as “is due”, “due date”, “deadline”, and “closes”, and Portuguese labels such as “está marcado(a) para esta data”, “Término de”, “Encerramento de”, “data limite”, and “prazo de entrega”. Opening events and generic meetings are excluded. This deliberately conservative label rule can miss custom names or other languages. The parser retains UID, summary, description, times, URL, and occurrence identity. Links come from `URL` or a description URL; absent or unsafe links fall back to Moodle's calendar.

`RRULE`, comma-separated `RDATE`/`EXDATE`, detached `RECURRENCE-ID` updates, `SEQUENCE`/modification ordering, `STATUS:CANCELLED`, and calendar `METHOD:CANCEL` are supported. Invalid individual events are skipped with a count; broken feeds produce a safe error and are retried. Recurrence expansion is bounded at 100,000 occurrences per event. Custom non-IANA `VTIMEZONE` definitions, period-valued `RDATE`, and `RANGE=THISANDFUTURE` are not supported; range overrides reject the cycle to avoid stale reminders.

Calendar mode uses stable positive synthetic course, assignment, and recipient identifiers compatible with the existing message contract. Identity includes the feed origin, UID, original recurring occurrence, deadline, and recipient; rotating the export token does not change it. Successful publications are remembered during the process lifetime. After a restart, the same message may be published again, and the notification service's existing persisted delivered-ID history suppresses duplicate emails. Keep its `notifications_data` volume. A changed deadline receives a new ID and can trigger a new reminder when eligible; text-only edits do not resend. Cancellation or disappearance suppresses future reminders, but cannot retract emails or messages already queued. Existing delivery retains its SMTP-send/history-write crash window.

iCalendar does not expose assignment submission state, enrolment roles, team settings, or whether submissions are enabled. Calendar reminders go only to `MOODLE_ICAL_EMAIL` and may include work already submitted. They are deadline reminders, not proof of incomplete work.

## Running and testing

Complete [Environment](../README.md#environment) before running the worker. The worker reads the process environment and does not load `.env` files itself.

From the repository root:

```sh
docker build -t noodle-monitoring:latest ./monitoring
docker run --rm --name noodle-monitoring --env-file .env noodle-monitoring:latest
```

Run the unit tests from `monitoring/`:

```sh
go test ./...
```

For an optional read-only live calendar check, provide `MOODLE_ICAL_URL` in the process environment and run `TEST_MOODLE_ICAL_LIVE=1 go test -run TestLiveCalendarIntegration -v`. It fetches and parses the feed without publishing messages. HTTP polling, failures, updates, and restart identity are also exercised using local test servers in the normal suite. With RabbitMQ running and `RABBITMQ_URL` configured, run `go test -tags=integration ./...` for the broker integration test.

Stop the process or container with `SIGINT` or `SIGTERM` for graceful shutdown. Moodle may limit which submissions are returned based on the authenticated account's permissions; an empty submissions array is logged only as an observed count, not interpreted as submission completion.

At the default `info` level, the worker logs major lifecycle events and one aggregate cycle summary. Moodle warnings are grouped by operation and warning code; warning groups that reduce monitoring coverage produce one aggregate warning instead of one line per inaccessible activity. Set `LOG_LEVEL=debug` to inspect individual course, assignment, and Moodle warning diagnostics.

## Personal API rules and Moodle limitations

Each worker authenticates one student's account and uses `core_webservice_get_site_info`, `core_enrol_get_users_courses`, `mod_assign_get_assignments`, and `mod_assign_get_submission_status`. The status call omits `userid`, using Moodle's authenticated-user default. No roster or bulk submission endpoint is called. The recipient is `NOTIFICATION_EMAIL`, with `MOODLE_ICAL_EMAIL` accepted as a compatibility fallback.

Only individual assignments with submissions enabled and a deadline in `(now, now + 24h]` are considered. A successful owner-specific latest attempt is required. `submitted` is complete; `new`, `draft`, and `reopened` are pending. A last attempt with no submission record is pending only when Moodle explicitly reports that submissions are enabled, the owner can edit, the attempt is unlocked, and it is not graded. Missing attempts, unknown states, mismatched owners, permission failures, or any status warning skip that assignment and report partial coverage.

Team submissions, locked/disabled/non-editable attempts, offline-graded empty attempts, and individual extensions are excluded. PUCRS Moodle 4.5 returns `extensionduedate: null` for no extension; null and zero are supported. Moodle only exposes activities visible to the signed-in account. Assignment warnings can therefore mean partial coverage even when accessible assignments are processed successfully. This student-specific integration cannot independently prove that an activity has identical visibility or deadline settings for every classmate.

Reminders retain the stable `course:assignment:student` identity. Polling or a worker restart can republish a message; the unchanged notification service's persistent SQLite history suppresses completed deliveries, including after its own restart. Changing an API deadline does not reset that identity. Keep `notifications_data`. The existing SMTP-send/history-write crash window remains; sent emails cannot be recalled, and SMTP retries stop after the configured bounded attempts.

The older bulk/roster clients remain available to the Bruno development collection and existing regression tests, but the running personal worker does not use them.

See [acceptance tests](../tests/acceptance/README.md) for the four recorded Mailpit scenarios. For a read-only real-site validation, supply the Moodle API credentials in the process environment and run `TEST_MOODLE_API_LIVE=1 go test -run TestLivePersonalMoodleIntegration -v`. This checks up to two upcoming assignments without publishing any messages; it fails clearly if no suitable live assignment is available. No live Moodle credentials are used by the automated acceptance stack.
