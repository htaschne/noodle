# Moodle setup

Use a test course with fictitious students, individual assignments visible to the
whole class, and one shared deadline. Disable group submissions and individual
extensions. The monitoring account discovers all its courses; enroll it only in
the course intended for the demonstration.

## Web services

In the installation's administration, enable web services and the REST protocol.
Create an enabled external service and authorize the monitoring account to use:

| Function | Purpose |
| --- | --- |
| `core_webservice_get_site_info` | Identify the authenticated account |
| `core_enrol_get_users_courses` | Discover its courses |
| `mod_assign_get_assignments` | Read assignments, deadlines and `cmid` |
| `core_enrol_get_enrolled_users` | Read students, role shortnames and emails |
| `mod_assign_get_submissions` | Read existing submission states |
| `mod_assign_get_submission_status` | Confirm students absent from the bulk response |

Give the account access to the course roster, student emails and other students'
submissions (including `mod/assign:viewgrades`). Check the exact capabilities and
function availability in **Site administration → Server → Web services → API
documentation** on the adopted installation. A student account generally cannot
inspect the whole class. Verify actual responses before the demonstration.

## Environment

Copy `.env.example` to `.env`, then set:

```dotenv
MOODLE_URL=https://your-test-moodle.example
MOODLE_USERNAME=monitoring-account
MOODLE_PASSWORD=account-password
MOODLE_SERVICE=external-service-shortname
MONITOR_INTERVAL=10s
```

`MOODLE_SERVICE` defaults to `moodle_mobile_app`. The worker obtains a fresh token
each cycle; it does not read `MOODLE_TOKEN`. Keep credentials out of Git and restore
`MONITOR_INTERVAL=1h` after the demonstration. Start the stack using the root guide.

## Students without attempts

An absent bulk submission is not sufficient evidence of pending work. Monitoring
requests that student's individual status. It includes the student only when
`lastattempt` explicitly permits the owner to edit, submissions are enabled,
the attempt is unlocked, and no individual extension is present. A missing
`submission` then means not started; an existing submission uses its returned
state. Only `new`, `draft` and `reopened` produce reminders.

The bulk warning `module / assignment ID / 3` is expected when no attempts exist;
it permits individual confirmation only for the matching empty assignment.
Other bulk warnings or missing assignment entries skip the assignment. Individual
errors, warnings or incomplete status data skip that student and report partial
coverage. Existing explicit bulk states retain their original classification.

Reference: [Moodle 4.5 assignment web services](https://github.com/moodle/moodle/blob/MOODLE_405_STABLE/mod/assign/externallib.php).
