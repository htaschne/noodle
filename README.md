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

Select IDs explicitly in **Collection Settings → Variables → Pre Request Vars** by setting `courseId` and `assignmentId`. Their defaults use `MOODLE_COURSE_ID` and `MOODLE_ASSIGNMENT_ID`; the collection never selects results automatically or stores credentials.
