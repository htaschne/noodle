# Moodle Monitoring Service

The monitoring service is a long-running worker that authenticates with Moodle, discovers the current user's courses and assignments, retrieves the submissions visible to that account, and repeats after `MONITOR_INTERVAL` (default `1h`).

## Configuration

The worker reads `MOODLE_URL`, `MOODLE_USERNAME`, `MOODLE_PASSWORD`, `MOODLE_SERVICE`, and `MONITOR_INTERVAL` from the process environment. It does not load `.env` files itself.

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
