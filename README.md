# Noodle 🍜 Notification Service for Moodle

Noodle monitors Moodle assignments and sends email reminders for eligible pending work. It consists of two independently buildable services connected through RabbitMQ:

| Service | Responsibility | Technology |
| --- | --- | --- |
| Monitoring | Discovers courses, filters assignments, evaluates student submission state, and publishes reminder requests | Go 1.27 |
| Notifications | Consumes reminder requests, sends email, and records delivered message IDs | Java 25, Spring Boot 4.1.2, SQLite |

RabbitMQ carries reminder messages. The local Docker Compose stack uses Mailpit as its SMTP server and email viewer. See [Architecture](docs/ARCHITECTURE.md) for the internal flow and reliability behavior.

## Development

### Prerequisites

#### Tools

Install [Docker](https://docs.docker.com/get-started/get-docker/). For development outside Docker, Monitoring requires Go 1.27, while Notifications requires Java 25 and Maven.

#### Environment

Create a local environment file:

```sh
cp .env.example .env
```

then add missing the empty fields with your credentials into `.env` you just created:

``` sh
MOODLE_USERNAME=
MOODLE_PASSWORD=
MOODLE_TOKEN=
```

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
