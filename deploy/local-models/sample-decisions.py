"""Public measurement of shared worker CPU, no credentials/config inspection."""
import datetime
import json
import signal
import subprocess
import time

running = True


def stop(_signum, _frame):
    global running
    running = False


signal.signal(signal.SIGTERM, stop)
started = time.monotonic()
while running and time.monotonic() - started < 600:
    sample = {"at": datetime.datetime.now(datetime.timezone.utc).isoformat()}
    try:
        value = subprocess.check_output(
            ["docker", "stats", "--no-stream", "--format", "{{.CPUPerc}}", "butler-lab-local-models-1"],
            stderr=subprocess.DEVNULL, timeout=8,
        ).decode("ascii").strip()
        sample["worker_cpu_percent"] = float(value.rstrip("%"))
    except (subprocess.SubprocessError, ValueError, UnicodeError):
        sample["measurement_unavailable"] = True
    print(json.dumps(sample), flush=True)
    for _ in range(10):
        if not running:
            break
        time.sleep(.2)
