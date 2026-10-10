"""Preparation only: download a pinned, public official checkpoint, never code."""
import json
import pathlib
import sys
from huggingface_hub import snapshot_download

revision = "914f7f89142e33e77833254d9c9b90c3cef7303b"
root = pathlib.Path(sys.argv[1])
destination = root / revision
snapshot_download(
    repo_id="google/embeddinggemma-2", revision=revision,
    local_dir=str(destination), token=False,
    allow_patterns=["*.json", "*.safetensors", "*.model", "*.jinja", "1_Pooling/*", "2_Normalize/*"],
    max_workers=2,
)
(destination / "butler-model.json").write_text(json.dumps({
    "model": "google/embeddinggemma-2", "revision": revision,
    "runtime_version": "torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0",
}), encoding="utf-8")
print("Pinned official EmbeddingGemma2 checkpoint prepared")
