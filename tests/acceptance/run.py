#!/usr/bin/env python3
"""Run isolated acceptance tests with fictional Moodle responses and Mailpit.

Does not load .env. Removes only the uniquely named test stack and its volumes.
"""
import os
import datetime
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
PROJECT = "noodle_acceptance_" + str(os.getpid())
COMPOSE = ["docker", "compose", "--env-file", os.devnull, "-p", PROJECT, "-f", str(Path(__file__).with_name("compose.yaml"))]


def run(*args, capture=False):
    return subprocess.run(list(args), cwd=ROOT, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def port(service, internal):
    return run(*COMPOSE, "port", service, str(internal), capture=True).strip().rsplit(":", 1)[1]


try:
    run(*COMPOSE, "up", "--build", "-d", "--wait", "--wait-timeout", "90")
    env = {k: v for k, v in os.environ.items()
           if not k.startswith(("MOODLE_", "TEST_MOODLE_", "RABBITMQ_", "SMTP_", "NOTIFICATION_"))}
    env.update({
        "RABBITMQ_URL": "amqp://noodle_test:noodle_test@127.0.0.1:" + port("rabbitmq", 5672) + "/",
        "ACCEPTANCE_MAILPIT_URL": "http://127.0.0.1:" + port("mailpit", 8025),
        "ACCEPTANCE_COMPOSE_PROJECT": PROJECT,
        "ACCEPTANCE_COMPOSE_FILE": str(Path(__file__).with_name("compose.yaml")),
    })
    result = subprocess.run(["go", "test", "-tags=acceptance", "-run", "^TestPersonalAcceptance$", "-count=1", "-v", "-timeout=120s"],
                            cwd=ROOT / "monitoring", env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    transcript = "Recorded at " + datetime.datetime.now(datetime.timezone.utc).isoformat() + "\n" + result.stdout
    print(transcript, flush=True)
    Path(__file__).with_name("last-run.log").write_text(transcript)
    if result.returncode:
        run(*COMPOSE, "logs", "--no-color", "notifications", "mailpit")
    sys.exit(result.returncode)
finally:
    subprocess.run(COMPOSE + ["down", "--volumes", "--remove-orphans"], cwd=ROOT, check=False)
