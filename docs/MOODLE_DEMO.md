# Self-hosted Moodle demonstration

Use the [README quick start](../README.md#quick-start). On the first start, Compose installs Moodle, seeds a fictitious course, and provides a read-only token file to Monitoring. The worker polls on startup and every minute. A one-off poll uses the same RabbitMQ queue and delivery history.

From the repository root, these commands run another poll, show recent logs, and stop the stack, respectively. The last command keeps the volumes.

```sh
docker compose -f compose.yaml -f compose.moodle.yaml run --rm --no-deps -e MONITOR_ONCE=1 monitoring
docker compose -f compose.yaml -f compose.moodle.yaml logs --tail=50 monitoring notifications
docker compose -f compose.yaml -f compose.moodle.yaml down
```

In Moodle, sign in as `admin` with `DEMO_MOODLE_ADMIN_PASSWORD` or as `alex`, `blair`, or `casey` with `DEMO_MOODLE_STUDENT_PASSWORD`. In RabbitMQ Management, use `DEMO_RABBITMQ_USER` and `DEMO_RABBITMQ_PASSWORD` when set, or the corresponding `RABBITMQ_*` values. Find `noodle.reminders` under **Queues and Streams**.

## Reuse an existing demo

The quick start uses the existing `noodle-moodle-local` Compose project and its volumes. Copy the three `DEMO_MOODLE_*_PASSWORD` values from `.env.moodle-demo-local` into the root `.env`. If you already have a RabbitMQ account in the root file, keep it and copy the demo's `RABBITMQ_USER`, `RABBITMQ_PASSWORD`, and `RABBITMQ_URL` to `DEMO_RABBITMQ_USER`, `DEMO_RABBITMQ_PASSWORD`, and `DEMO_RABBITMQ_URL`, respectively. If you are setting up the root file for the first time, copy the RabbitMQ values using their original names.

For another existing demo, such as `noodle-moodle-video`, use that project's matching values in the same root `.env` and pass its name with `docker compose -p noodle-moodle-video …`. Use one demo project at a time because the browser ports are fixed.

## Record the demo

The **Noodle Demonstration Course** has these assignments. Deadlines are relative to the project's first install.

| Assignment | Initial deadline | Initial reminders |
| --- | --- | --- |
| Pending essay | In 2 hours | Alex never started and Casey has a draft. Blair already submitted. |
| Completed worksheet | In 3 hours | None. All three submitted. |
| Future project | In 48 hours | None. |
| Expired exercise | 1 hour ago | None. |

If the demo is running, stop it using the `down` command above. Use an unused Compose project name for fresh deadlines and an empty inbox. The recording uses the same root `.env`.

```sh
demo() { docker compose -p noodle-moodle-recording -f compose.yaml -f compose.moodle.yaml "$@"; }
demo up --build -d --wait --scale monitoring=0
demo stop notifications
demo run --rm --no-deps -e MONITOR_ONCE=1 monitoring
```

1. Show the assignments and filter **Participants** by the **Student** role. As Blair, open **Pending essay** and show **Submitted for grading**. The poll reports three students, two eligible assignments, and two published reminders.
2. Show **2 Ready** on `noodle.reminders` in RabbitMQ Management. Run `demo start notifications`.
3. Show the two **Pending essay** emails in Mailpit, addressed to `alex@example.test` and `casey@example.test`. Follow the Moodle assignment link in one message.
4. Run the one-off poll again. Check `demo logs --tail=30 notifications` for two `duplicate_reminder` entries and verify that Mailpit still has two messages. Run `demo down` to stop the recording stack and retain its volumes.

### Optional SMTP retry scene

With two reminders queued and Notifications stopped in a fresh recording project, configure Mailpit to return SMTP `451`.

```sh
curl -fsS -X PUT http://localhost:18025/api/v1/chaos \
  -H 'Content-Type: application/json' -d '{"Sender":{"ErrorCode":451,"Probability":100}}'
demo start notifications
demo logs -f notifications
```

When `notification_delivery_retry` appears, restore SMTP within the 20-second retry delay using a second terminal.

```sh
curl -fsS -X PUT http://localhost:18025/api/v1/chaos \
  -H 'Content-Type: application/json' -d '{}'
```

Show `notification_delivered` in the logs and both emails in Mailpit.
