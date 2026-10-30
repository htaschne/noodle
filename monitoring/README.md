# Moodle Monitoring Service

The monitoring service is a long-running worker that authenticates with Moodle, discovers the current user's courses, finds individual assignments due in the next 24 hours, and identifies enrolled students with explicit incomplete submission states. It only creates in-memory candidate records and does not send email, publish messages, or modify Moodle data. The cycle repeats after `MONITOR_INTERVAL` (default `1h`).

## Configuration

The worker reads configuration from the process environment. It does not load `.env` files itself.

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `MOODLE_URL` | Yes | — | Moodle base URL |
| `MOODLE_USERNAME` | Yes | — | Web-service account username |
| `MOODLE_PASSWORD` | Yes | — | Web-service account password |
| `MOODLE_SERVICE` | No | `moodle_mobile_app` | Moodle external-service name |
| `MONITOR_INTERVAL` | No | `1h` | Positive Go duration between cycles |
| `LOG_LEVEL` | No | `info` | Logging threshold: `debug`, `info`, `warn`, or `error` |

For local development, export the root configuration before starting the worker:

```sh
cd monitoring
set -a
. ../.env
set +a
go run .
```

From the repository root, build and run the container with:

```sh
docker build -t noodle-monitoring ./monitoring
docker run --rm --env-file .env noodle-monitoring
```

Stop the process or container with `SIGINT` or `SIGTERM` for graceful shutdown. Moodle may limit which submissions are returned based on the authenticated account's permissions; an empty submissions array is logged only as an observed count, not interpreted as submission completion.

At the default `info` level, the worker logs major lifecycle events and one aggregate cycle summary. Moodle warnings are grouped by operation and warning code; warning groups that reduce monitoring coverage produce one aggregate warning instead of one line per inaccessible activity. Set `LOG_LEVEL=debug` to inspect individual course, assignment, and Moodle warning diagnostics.

## Candidate rules and Moodle limitations

An assignment is eligible only when its standard deadline is in the UTC interval `(now, now + 24h]`, it is not a team assignment, and submissions are enabled. Individual deadline extensions are outside this phase.

Student roles are identified by Moodle's semantic role shortname (`student`), not by installation-specific numeric role IDs. Standard staff shortnames (`manager`, `coursecreator`, `editingteacher`, and `teacher`) are excluded. Unrecognized custom roles are treated as ambiguous and are not included.

For submissions, `submitted` is complete. Explicit latest-attempt states `new`, `draft`, and `reopened` are incomplete; unknown statuses, conflicting statuses for the same attempt, and users without a submission record are not classified. A returned assignment entry with an empty submission list is a successful empty result but produces no candidates. A missing assignment entry, API failure, or any Moodle warning makes that assignment's submission data unavailable, so the service logs the limitation and skips it. This fail-closed behavior means the web-service account must be allowed to view complete per-student submission states before downstream notification delivery can be added.
