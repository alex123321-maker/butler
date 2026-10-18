#!/usr/bin/env python3
"""Lab-only lossless final schedule export; invalid/partial captures cannot
replace the already retained full96 initial denominator. No model calls.
"""
import hashlib
import json
import os
import pathlib
import re
import sys


def digest(value):
    # MemoryInputDigest uses encoding/json field order rather than sorted keys;
    # structural identity and the collector's exact digest are both retained.
    return hashlib.sha256(value).hexdigest()


def export(proof):
    initial = json.loads((proof / "scheduled-before-auth.json").read_bytes())
    inputs = json.loads((proof / "inputs.json").read_bytes())["inputs"]
    measured = json.loads((proof / "measurements.json").read_bytes())["observations"]
    key = lambda value: (value["case_id"], value["arm"])
    expected = {key(value): value for value in initial["inputs"]}
    if len(expected) != 96 or len(inputs) != 96 or len(measured) != 96:
        raise SystemExit("full96 final denominator required")
    provided = {key(value): value for value in inputs}
    observed = {key(value): value for value in measured}
    if set(provided) != set(expected) or set(observed) != set(expected) or len(provided) != 96 or len(observed) != 96:
        raise SystemExit("full96 exact original identities required")
    for identity, original in expected.items():
        actual, observation = provided[identity], observed[identity]
        for field in ("question", "as_of_utc", "task", "instructions"):
            if actual[field] != original[field]:
                raise SystemExit("frozen original input differs")
        # Match Go's JSON bytes exactly, including fixed struct field order and
        # HTML escaping; the full input receipt is the binding being verified.
        encoded = json.dumps(actual, ensure_ascii=False, separators=(",", ":")).replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e").replace("\u2028", "\\u2028").replace("\u2029", "\\u2029").encode()
        if observation["input_sha256"] != digest(encoded) or observation["status"] not in ("completed", "failed"):
            raise SystemExit("final original digest/status differs")
    result = {"format": "butler.adaptive.full96.actual.v5", "inputs": inputs, "observations": measured}
    target = proof / "all-original-outcomes.complete.json"
    with os.fdopen(os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(json.dumps(result, ensure_ascii=False, separators=(",", ":")).encode()+b"\n")
        out.flush()
        os.fsync(out.fileno())
    os.replace(target, proof / "all-original-outcomes.json")
    return {"full_originals": 96, "bound_inputs": 96, "completed": sum(o["status"] == "completed" for o in measured), "failed": sum(o["status"] == "failed" for o in measured)}


if __name__ == "__main__":
    proof = pathlib.Path(sys.argv[1]).resolve()
    if not re.fullmatch(r"/srv/storage/butler/agents/issue87-native-[A-Za-z0-9_-]+/reports/native-v5", str(proof)):
        raise SystemExit("bounded final outcome reference required")
    print(json.dumps(export(proof)))
