# Tests

Run the unit tests from the repository root:

```sh
(cd monitoring && go test ./...)
(cd notifications && mvn test)
```

| Service | Coverage |
| --- | --- |
| Monitoring | Deadline window, roles, submission states, individual confirmation of missing attempts, permission failures and message serialization |
| Notifications | Payload validation, email content, duplicate suppression, persisted history, bounded SMTP retries and acknowledgment |

## Recorded run: 2026-11-18

`go test ./...` passed. `mvn test` passed all 19 tests, with no failures or skips.
These results do not validate permissions or responses on the partner's Moodle
installation; see [Moodle setup](moodle.md).
