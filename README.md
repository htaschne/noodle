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


## API 

The version-controlled collection is in [`bruno/`](./bruno). Request fields reference process environment variables directly with `{{process.env.MOODLE_*}}`. No credentials are stored in the collection.

### Install Bruno

For the desktop application, download Bruno from the [official downloads page](https://www.usebruno.com/downloads) or install it on macOS with:


### Configure Moodle access

From the repository root:

```sh
cp .env.example .env
```

Fill in `.env` with your Moodle username and password or an existing token, plus the course and assignment IDs needed by the requests. `MOODLE_SERVICE` normally remains `moodle_mobile_app`. The `.env` file is ignored by Git.

`MOODLE_TOKEN` is required by the four authenticated web-service requests. If it is empty, run **Get Token**, save the returned token in your local `.env`, and restart Bruno through the launcher before running the other requests. Bruno does not write the token back to `.env`.

The repository-root `.env` is not inside the Bruno collection, so do not rely on Bruno to discover it automatically. Use the launcher below to export its values into the Bruno process. The launcher treats each non-comment line as a literal `NAME=value` pair, does not execute the file, and does not print its contents.

With the official Docker CLI image, run from the repository root:

> Yet to be tested

```sh
docker run --rm --env-file .env \
  -v "$PWD/bruno:/collection" \
  -w /collection \
  usebruno/cli:latest run
```
