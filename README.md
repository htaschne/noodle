# Noodle 🍜 Notification Service for Moodle

Noodle (Notification Service for Moodle) is composed of independently deployable services. The Bruno collection in this repository is a development tool for exploring Moodle's REST API; it is not a runtime dependency of the application services.

## Docs

Click [here](./docs/ARCHITECTURE.md) to check our architecture decisions

## Development

### Prerequisites

#### Part 1 — Tools

Install [Docker](https://docs.docker.com/get-started/get-docker/). For development outside Docker, Monitoring requires Go 1.27, while Notifications requires Java 25 and Maven.

#### Part 2 — Environment

Create the local environment file and fill in the blank values:

```sh
cp .env.example .env
```

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `MOODLE_URL` | Yes | — | Moodle base URL |
| `MOODLE_USERNAME` | Yes | — | Moodle web-service username |
| `MOODLE_PASSWORD` | Yes | — | Moodle web-service password |
| `MOODLE_SERVICE` | No | `moodle_mobile_app` | Moodle external-service name |
| `MONITOR_INTERVAL` | No | `1h` | Monitoring interval as a positive Go duration |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, or `error` |
| `RABBITMQ_USER` | For local RabbitMQ | `noodle` | Local broker username |
| `RABBITMQ_PASSWORD` | For local RabbitMQ | — | Local broker password |
| `RABBITMQ_URL` | When candidates are published | — | Complete authenticated AMQP URL used by Monitoring; URL-encode credentials |
| `SMTP_HOST` | No | `localhost` | SMTP host used outside Compose |
| `SMTP_PORT` | No | `1025` | SMTP port used outside Compose |
| `SMTP_FROM` | No | `noodle@example.test` | Reminder sender address |

Do not commit `.env`.

### Running Noodle

Start, follow logs, and stop the local stack:

```sh
docker compose up --build -d
docker compose logs -f
docker compose down
```

Build either service independently:

```sh
docker build -t noodle-monitoring:latest ./monitoring
docker build -t noodle-notifications:latest ./notifications
```

Each application service owns its build, Dockerfile, and image. Mailpit is available at [http://localhost:8025](http://localhost:8025).

Notifications records delivered reminder IDs in SQLite after SMTP succeeds. A crash between those operations can cause a duplicate email when RabbitMQ redelivers the message; delivery is not exactly once.
After three failed SMTP attempts, a reminder is rejected without requeue; no dead-letter queue is configured yet.

## RabbitMQ

The management interface is available at [http://localhost:15672](http://localhost:15672). AMQP is exposed at `localhost:5672`. Both ports are bound only to the local machine.

Independently running Go services connect to these endpoints:

- From the host: `amqp://localhost:5672`
- From another container on the Compose network: `amqp://rabbitmq:5672`

These are host and port endpoints, not complete authenticated connection strings. Services must supply the configured credentials separately and must not embed passwords in source code or documentation.

## API testing with Bruno

After completing Prerequisites Part 2, install [Bruno](https://www.usebruno.com/downloads), export the root `.env` values into the Bruno process, and open [`bruno/`](./bruno). Run **Get Token** → **Site Info** → **Get My Courses** → **Get Assignments** → **Get Submissions**; run **Get Enrolled Users** after selecting a course.

When a request needs a course or assignment ID, Bruno Desktop prompts for it. Copy the ID from the preceding response; the collection does not select results automatically or store credentials.
