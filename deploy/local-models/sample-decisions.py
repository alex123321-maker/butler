"""Public measurement of shared worker CPU, no credentials/config inspection."""
import datetime
import json
import signal
import subprocess
import sys
import time

running = True


def stop(_signum, _frame):
    global running
    running = False


signal.signal(signal.SIGTERM, stop)
started = time.monotonic()
container = sys.argv[1] if len(sys.argv) >= 2 else 'butler-lab-local-models-1'
while running and time.monotonic() - started < 600:
    sample = {"at": datetime.datetime.now(datetime.timezone.utc).isoformat()}
    try:
        value = subprocess.check_output(
            ["docker", "stats", "--no-stream", "--format", "{{.CPUPerc}}", container],
            stderr=subprocess.DEVNULL, timeout=8,
        ).decode("ascii").strip()
        sample["worker_cpu_percent"] = float(value.rstrip("%"))
        if len(sys.argv) >= 3:
            qwen = subprocess.check_output(['docker','stats','--no-stream','--format','{{.CPUPerc}}',sys.argv[2]],stderr=subprocess.DEVNULL,timeout=8).decode('ascii').strip()
            sample['qwen_cpu_percent'] = float(qwen.rstrip('%'))
    except (subprocess.SubprocessError, ValueError, UnicodeError):
        sample["measurement_unavailable"] = True
    print(json.dumps(sample), flush=True)
    for _ in range(10):
        if not running:
            break
        time.sleep(.2)
