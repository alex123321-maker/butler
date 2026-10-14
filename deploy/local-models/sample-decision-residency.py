"""Whole-device GPU observations at 250 ms; no prompt or credential reads."""
import datetime
import json
import signal
import subprocess
import time

running = True
def stop(*_):
    global running
    running = False
signal.signal(signal.SIGTERM, stop)
started = time.monotonic()
while running and time.monotonic()-started < 900:
    sample = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat()}
    try:
        line = subprocess.check_output(['nvidia-smi', '--query-gpu=memory.total,memory.used,memory.free', '--format=csv,noheader,nounits'], timeout=2).decode().strip()
        total, used, free = map(int,line.split(','))
        sample.update(total_mib=total,used_mib=used,free_mib=free,reserve_ok=used<=5632 and free>=512)
        processes = subprocess.check_output(['nvidia-smi','--query-compute-apps=pid,used_gpu_memory','--format=csv,noheader,nounits'],timeout=2).decode().strip()
        sample['compute_processes'] = processes
    except (ValueError,subprocess.SubprocessError):
        sample['telemetry_unavailable'] = True
    print(json.dumps(sample),flush=True)
    time.sleep(.25)
