"""Preparation only: download a pinned, public official checkpoint, never code."""
import json
import pathlib
import sys
from huggingface_hub import snapshot_download

root = pathlib.Path(sys.argv[1])
catalog = pathlib.Path(__file__).with_name("models.json")
for profile in json.loads(catalog.read_text())["models"]:
    revision=profile["revision"]
    if len(revision)!=40 or any(c not in "0123456789abcdef" for c in revision):
        raise ValueError("full revision required")
    destination=root/revision
    snapshot_download(
        repo_id=profile["model"],revision=revision,local_dir=str(destination),token=False,
        allow_patterns=["*.json","*.safetensors","*.model","*.jinja","*.txt","LICENSE*","README.md","1_Pooling/*","2_Normalize/*","encoder_config/*"],
        max_workers=2,
    )
    (destination/"butler-model.json").write_text(json.dumps({
        "model":profile["model"],"revision":revision,"runtime_version":profile["runtimeVersion"],
    }),encoding="utf-8")
    print("Pinned public checkpoint prepared:",profile["model"],revision)
