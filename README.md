# Noodle 🍜 Notification Service for Moodle

Noodle (Notification Service for Moodle) is composed of independently deployable Go microservices. The Bruno collection in this repository is a development tool for exploring Moodle's REST API; it is not a runtime dependency of either service.

## Docs

Click [here](./docs/ARCHITECTURE.md) to check our architecture decisions

## Development

### Prerequisites

#### Part 1 — Tools

Install [Docker](https://docs.docker.com/get-started/get-docker/). Install Go 1.27 when running or testing Monitoring outside Docker.

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
| `RABBITMQ_URL` | When candidates are published | — | Complete authenticated AMQP URL used by Monitoring |

Do not commit `.env`.

### Running each service

Build and run the Monitoring service:

```sh
docker build -t noodle-monitoring:latest ./monitoring
docker run --rm --name noodle-monitoring --env-file .env noodle-monitoring:latest
```

Each Go microservice owns its Go module, Dockerfile, and image. Docker Compose is only used for local infrastructure and does not make Monitoring depend on RabbitMQ at build time.

## RabbitMQ

After completing Prerequisites Part 2, start the local broker:

```sh
docker compose up -d rabbitmq
```

Check its container and broker health:

```sh
docker compose ps
docker compose exec rabbitmq rabbitmq-diagnostics -q ping
```

The management interface is available at [http://localhost:15672](http://localhost:15672). AMQP is exposed at `localhost:5672`. Both ports are bound only to the local machine.

Independently running Go services connect to these endpoints:

- From the host: `amqp://localhost:5672`
- From another container on the Compose network: `amqp://rabbitmq:5672`

These are host and port endpoints, not complete authenticated connection strings. Services must supply the configured credentials separately and must not embed passwords in source code or documentation.

Stop RabbitMQ while preserving its named data volume:

```sh
docker compose down
```

To intentionally remove the persisted broker data as well:

```sh
docker compose down --volumes
```

## API testing with Bruno

After completing Prerequisites Part 2, install [Bruno](https://www.usebruno.com/downloads), export the root `.env` values into the Bruno process, and open [`bruno/`](./bruno). Run **Get Token** → **Site Info** → **Get My Courses** → **Get Assignments** → **Get Submissions**; run **Get Enrolled Users** after selecting a course.

When a request needs a course or assignment ID, Bruno Desktop prompts for it. Copy the ID from the preceding response; the collection does not select results automatically or store credentials.
