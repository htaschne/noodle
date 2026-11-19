# Acceptance tests

Run from the repository root with Docker Desktop and Go available:

```sh
python3 tests/acceptance/run.py
```

The runner creates a uniquely named isolated RabbitMQ, Notifications, and Mailpit stack. It does not load `.env` or use real Moodle credentials. Ports are allocated on loopback. A fictional Moodle HTTP server exercises the production personal monitoring cycle; unneeded roster and bulk-submission endpoints are rejected. The runner removes only its own containers, network, and test-history volume on exit. The transcript is saved to [last-run.log](last-run.log).

## Recorded results: 2026-11-19

| Project acceptance criterion | Verified behavior |
| --- | --- |
| 1. Pending task due within 24 hours | A never-started assignment produced one captured email containing its name, deadline, and Moodle activity link. |
| 2. No reminder for submitted or out-of-window work | Submitted, future, overdue, and group assignments produced no additional email. Unit tests also cover missing deadlines and disabled submissions. |
| 3. No new send for a completed request | Replaying the request after recreating the monitoring client and restarting Notifications kept the email count at one. SQLite history survived the restart. |
| 4. Log failures and retry | Mailpit Chaos rejected SMTP with 451. Notifications logged attempt 1; after the rejection was removed, its bounded retry delivered the draft reminder. |

The successful run captured exactly two emails, both for the fictional `student@example.test` account. No real email was delivered and no live Moodle request was made by this suite.

Other validation includes `go test -race ./...`, `go vet ./...`, all 19 Java notification tests, and both service image builds. The optional read-only `TestLivePersonalMoodleIntegration` verifies the authenticated student's status on up to two real upcoming PUCRS assignments without publishing messages. The live check and fake-Moodle acceptance suite establish different things: actual account/API compatibility and a controlled full notification flow, respectively.

The existing broker-only integration test can also run with `RABBITMQ_URL` configured: `go test -tags=integration ./...` from `monitoring/`. It expects a test queue with no consuming notification worker because it checks queue-depth growth.

## Demonstration

The verbose transcript names each criterion as it passes. For a presentation, rerun the command and show the four PASS lines. All generated addresses and credentials are fictional. The isolated stack is cleaned up automatically; the normal local stack's Mailpit UI remains available at `http://localhost:8025` when running `docker compose up --build -d`.

## Limits

The controlled Moodle responses are test doubles, not a real Moodle test course. The personal deployment model is one student's account per worker. Course-wide visibility, institutional policy, and permission changes remain dependent on the Moodle installation. Calendar-only mode cannot satisfy the submitted-work exclusion. The SMTP-send/history-write crash window is documented in [Architecture](../../docs/ARCHITECTURE.md).
