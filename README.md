# Noodle 🍜 Notification Service for Moodle

Noodle monitors Moodle assignments and sends email reminders for eligible pending work. It consists of two independently buildable services connected through RabbitMQ:

| Service | Responsibility | Technology |
| --- | --- | --- |
| Monitoring | Reads calendar deadlines or evaluates assignments and submission state through the API, then publishes reminder requests | Go 1.27 |
| Notifications | Consumes reminder requests, sends email, and records delivered message IDs | Java 25, Spring Boot 4.1.2, SQLite |

RabbitMQ carries reminder messages. The local Docker Compose stack uses Mailpit as its SMTP server and email viewer. See [Architecture](docs/ARCHITECTURE.md) for the internal flow and reliability behavior.

Each student authorizes their own Moodle account. In personal API mode, the worker reads that student's courses and assignments, checks their own latest submission status, and sends reminders only to their configured email. It does not read class rosters or classmates' submissions. One worker is configured per student; shared account registration and credential storage are outside the current implementation.

> [!WARNING]
> **Calendar-only limitation:** iCalendar exports contain deadlines but no submission status, so calendar-only reminders may include work already submitted. Use `MOODLE_SOURCE=api` for submission-aware reminders. Calendar mode also cannot verify team-assignment settings, and a changed calendar deadline can generate another reminder for the same event. See [monitoring behavior and limitations](monitoring/README.md).

The four project acceptance scenarios are automated against fictional Moodle REST responses, real RabbitMQ and the Notifications service, and Mailpit. See [test results and reproduction steps](tests/acceptance/README.md). Read-only PUCRS checks also validated the personal API client; production permissions and hidden activities can still limit coverage.

The [presentation and demonstration script](docs/PRESENTATION.md) covers the implementation, the four acceptance scenarios, and the documented limitations.

## Self-hosted Moodle demo

Run `./scripts/moodle-demo up` to bring up a real local Moodle, a fictitious course, and course-wide reminders through Mailpit. See the [demo guide](docs/MOODLE_DEMO.md) for sign-in, polling, and recording steps.

## Development

### Prerequisites

#### Tools

Install [Docker](https://docs.docker.com/get-started/get-docker/). For development outside Docker, Monitoring requires Go 1.27, while Notifications requires Java 25 and Maven.

#### Environment

Create a local environment file:

```sh
cp .env.example .env
```

For submission-aware personal reminders, configure your own Moodle account and email:

```dotenv
MOODLE_SOURCE=api
MOODLE_URL=https://moodle.pucrs.br
MOODLE_USERNAME=<your Moodle username>
MOODLE_PASSWORD=<your Moodle password>
MOODLE_SERVICE=moodle_mobile_app
NOTIFICATION_EMAIL=<your reminder email>
MONITOR_INTERVAL=1h
```

Personal API mode uses `mod_assign_get_submission_status` for the authenticated student. No access to classmates' emails or submissions is needed. API authentication uses the configured username and password by default. Set `MOODLE_TOKEN_FILE` to use an existing token from a mounted file.

For calendar-only reminders, follow the export steps in `.env.example`, set `MOODLE_SOURCE=ical`, `MOODLE_ICAL_URL`, and `MOODLE_ICAL_EMAIL` (or `NOTIFICATION_EMAIL`). With `MOODLE_SOURCE=auto` or unset, a configured calendar URL takes precedence; otherwise the personal API is used. Explicit `api` mode preserves a saved calendar URL while selecting submission-aware monitoring. API failures do not silently switch to calendar-only reminders.

The calendar URL contains a secret authentication token. Keep it and your Moodle credentials only in the ignored `.env` file. See [calendar configuration](monitoring/README.md#calendar-configuration).

### Run

Start the local stack:

```sh
docker compose up --build -d
docker compose ps
docker compose logs -f
```

Stop it without removing persisted RabbitMQ or notification data:

```sh
docker compose down
```

Local interfaces:

- Mailpit: [http://localhost:8025](http://localhost:8025)
- RabbitMQ management: [http://localhost:15672](http://localhost:15672)
- RabbitMQ AMQP: `localhost:5672`

All published ports are bound to `127.0.0.1`.

### Build and test

Build either service independently:

```sh
docker build -t noodle-monitoring:latest ./monitoring
docker build -t noodle-notifications:latest ./notifications
```

Run service tests with the local toolchains:

```sh
(cd monitoring && go test ./...)
(cd notifications && mvn test)
```

## Moodle API inspection

The optional [Bruno collection](bruno/) mirrors the Moodle calls used by Monitoring for development and troubleshooting. It is not a runtime dependency.
