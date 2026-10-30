# Noodle 🍜 Notification Service for Moodle

Noodle (Notification Service for Moodle) is composed of independently deployable services. The Bruno collection in this repository is a development tool for exploring Moodle's REST API; it is not a runtime dependency of the application services.

## Docs

Click [here](./docs/ARCHITECTURE.md) to check our architecture decisions

## Development

### Prerequisites

#### Tools

Install [Docker](https://docs.docker.com/get-started/get-docker/). For development outside Docker, Monitoring requires Go 1.27, while Notifications requires Java 25 and Maven.

#### Environment

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
| `RABBITMQ_USER` | No | `noodle` | Local broker username |
| `RABBITMQ_PASSWORD` | No | `noodle_secret_password` | Local broker password |
| `RABBITMQ_URL` | No | `amqp://{RABBITMQ_USER}:{RABBITMQ_PASSWORD}@localhost:5672` | Complete authenticated AMQP URL used by Monitoring; URL-encode credentials |
| `SMTP_HOST` | No | `localhost` | SMTP host used outside Compose |
| `SMTP_PORT` | No | `1025` | SMTP port used outside Compose |
| `SMTP_FROM` | No | `noodle@example.test` | Reminder sender address |


### Running Noodle

Start, follow logs, and stop the local stack:

```sh
# build
docker compose up --build -d

# watch the logs
docker compose logs -f

# takes down the service
docker compose down
```

Build either service independently:

```sh
docker build -t noodle-monitoring:latest ./monitoring
docker build -t noodle-notifications:latest ./notifications
```

### Watch the services via web
Each application service owns its build, Dockerfile, and image.

Mailpit is available at [http://localhost:8025](http://localhost:8025).

RabbitMQ is available at [http://localhost:15672](http://localhost:15672). AMQP is exposed at `localhost:5672`. Both ports are bound only to the local machine.

## API testing with Bruno

After completing Prerequisites Part 2, install [Bruno](https://www.usebruno.com/downloads), export the root `.env` values into the Bruno process, and open [`bruno/`](./bruno). Run **Get Token** → **Site Info** → **Get My Courses** → **Get Assignments** → **Get Submissions**; run **Get Enrolled Users** after selecting a course.

When a request needs a course or assignment ID, Bruno Desktop prompts for it. Copy the ID from the preceding response; the collection does not select results automatically or store credentials.
