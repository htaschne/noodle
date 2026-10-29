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

## API testing with Bruno

Install [Bruno](https://www.usebruno.com/downloads), then configure the root `.env`:

```sh
cp .env.example .env
```

Export the root `.env` values into the Bruno process and open [`bruno/`](./bruno). Run **Get Token** → **Site Info** → **Get My Courses** → **Get Assignments** → **Get Submissions**; run **Get Enrolled Users** after selecting a course.

When a request needs a course or assignment ID, Bruno Desktop prompts for it. Copy the ID from the preceding response; the collection does not select results automatically or store credentials.
