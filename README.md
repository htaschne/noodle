# Noodle 🍜 Notification Service for Moodle

Noodle (Notification Service for Moodle) is composed of independently deployable Go microservices. The Bruno collection in this repository is a development tool for exploring Moodle's REST API; it is not a runtime dependency of either service.

## Docs

Click [here](./docs/ARCHITECTURE.md) to check our architecture decisions

## Development

### Prerequisites

[Docker](https://docs.docker.com/get-started/get-docker/) is installed

### Running each service

To run the Moodle Monitoring Service run:

```sh
docker build -t moodle-monitoring ./monitoring
```

Each Go microservice owns its Go module, Dockerfile, and image. Docker Compose is only used for local infrastructure and does not make Monitoring depend on RabbitMQ at build time.

## RabbitMQ

Set `RABBITMQ_USER` and `RABBITMQ_PASSWORD` in the root `.env`, then start the local broker:

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

Install [Bruno](https://www.usebruno.com/downloads), then configure the root `.env`:

```sh
cp .env.example .env
```

Export the root `.env` values into the Bruno process and open [`bruno/`](./bruno). Run **Get Token** → **Site Info** → **Get My Courses** → **Get Assignments** → **Get Submissions**; run **Get Enrolled Users** after selecting a course.

When a request needs a course or assignment ID, Bruno Desktop prompts for it. Copy the ID from the preceding response; the collection does not select results automatically or store credentials.
