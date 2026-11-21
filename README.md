# Noodle 🍜 Notification Service for Moodle

Noodle monitors Moodle assignments and sends email reminders for upcoming deadlines.

**[Watch the demo](<https://youtu.be/7tYmvFsaAiE>)**

## Quick start

Run Docker with Compose 2.24.4 or later. [Docker installation guide](https://docs.docker.com/compose/install/)

From the repository root, create `.env` if you do not already have one.

```sh
test -e .env || (umask 077 && cp .env.example .env)
```

Fill in `RABBITMQ_PASSWORD` and the three `DEMO_MOODLE_*_PASSWORD` values in `.env`. For a previously installed demo, carry over its existing passwords. [Credential migration](docs/MOODLE_DEMO.md#reuse-an-existing-demo)

```sh
docker compose -f compose.yaml -f compose.moodle.yaml up --build -d --wait
```

- [Moodle](http://moodle.localhost:18080) uses `admin` and `DEMO_MOODLE_ADMIN_PASSWORD`.
- [RabbitMQ Management](http://localhost:25672) uses the RabbitMQ account configured in `.env`.
- [Mailpit](http://localhost:18025) shows captured reminder emails.

[Demo recording guide](docs/MOODLE_DEMO.md) · [Personal Moodle and calendar setup](monitoring/README.md#running-and-testing)
