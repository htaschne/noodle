# Moodle Monitoring Service

The monitoring service is a long-running worker that authenticates with Moodle, discovers the current user's courses, finds individual assignments due in the next 24 hours, identifies enrolled students with explicit incomplete submission states, and publishes reminder candidates to RabbitMQ. It does not send email or modify Moodle data. The cycle repeats after `MONITOR_INTERVAL` (default `1h`).

## Running and testing

Complete [Prerequisites Part 2](../README.md#part-2--environment) before running the worker. The worker reads the process environment and does not load `.env` files itself.

From the repository root:

```sh
docker build -t noodle-monitoring:latest ./monitoring
docker run --rm --name noodle-monitoring --env-file .env noodle-monitoring:latest
```

Run the unit tests from `monitoring/`:

```sh
go test ./...
```

Stop the process or container with `SIGINT` or `SIGTERM` for graceful shutdown. Moodle may limit which submissions are returned based on the authenticated account's permissions; an empty submissions array is logged only as an observed count, not interpreted as submission completion.

At the default `info` level, the worker logs major lifecycle events and one aggregate cycle summary. Moodle warnings are grouped by operation and warning code; warning groups that reduce monitoring coverage produce one aggregate warning instead of one line per inaccessible activity. Set `LOG_LEVEL=debug` to inspect individual course, assignment, and Moodle warning diagnostics.

## Candidate rules and Moodle limitations

An assignment is eligible only when its standard deadline is in the UTC interval `(now, now + 24h]`, it is not a team assignment, and submissions are enabled. Individual deadline extensions are outside this phase.

Student roles are identified by Moodle's semantic role shortname (`student`), not by installation-specific numeric role IDs. Standard staff shortnames (`manager`, `coursecreator`, `editingteacher`, and `teacher`) are excluded. Unrecognized custom roles are treated as ambiguous and are not included.

For submissions, `submitted` is complete. Explicit latest-attempt states `new`, `draft`, and `reopened` are incomplete; unknown statuses, conflicting statuses for the same attempt, and users without a submission record are not classified. A returned assignment entry with an empty submission list is a successful empty result but produces no candidates. A missing assignment entry, API failure, or any Moodle warning makes that assignment's submission data unavailable, so the service logs the limitation and skips it. This fail-closed behavior means the web-service account must be allowed to view complete per-student submission states before candidates can be published.
