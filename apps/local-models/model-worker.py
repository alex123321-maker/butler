"""Exception-only tensor runtime; Go owns auth, queueing, deadlines and process life.

No remote model code, network input, tool execution, task state or raw error output.
The single model is loaded from an operator-prepared read-only pinned directory.
"""
import contextlib
import json
import os
import pathlib
import sys

os.environ["HF_HUB_OFFLINE"] = "1"
os.environ["TRANSFORMERS_OFFLINE"] = "1"
os.environ["HF_HUB_DISABLE_TELEMETRY"] = "1"
os.environ["TOKENIZERS_PARALLELISM"] = "false"

def emit(value):
    sys.stdout.write(json.dumps(value, ensure_ascii=False, allow_nan=False) + "\n")
    sys.stdout.flush()

try:
    init = json.loads(sys.stdin.readline(65536))
    if init.get("op") != "init":
        raise ValueError("invalid init")
    profile = init["profile"]
    if profile["model"] != "google/embeddinggemma-2" or profile["runtime_version"] != "torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0":
        raise ValueError("unsupported profile")
    revision = profile["revision"]
    if len(revision) != 40 or any(c not in "0123456789abcdef" for c in revision):
        raise ValueError("invalid revision")
    path = pathlib.Path(os.environ["BUTLER_MODEL_ROOT"]) / revision
    metadata = json.loads((path / "butler-model.json").read_text())
    if metadata["model"] != profile["model"] or metadata["revision"] != revision or metadata["runtime_version"] != profile["runtime_version"]:
        raise ValueError("model metadata mismatch")
    with contextlib.redirect_stdout(sys.stderr):
        import torch
        import transformers
        import sentence_transformers
        from sentence_transformers import SentenceTransformer
        if torch.__version__ != "2.10.0+cu128" or transformers.__version__ != "5.19.0" or sentence_transformers.__version__ != "6.1.0":
            raise ValueError("library version mismatch")
        device = init["device"]
        if device not in ("cpu", "gpu"):
            raise ValueError("unsupported device")
        if device == "gpu" and (not torch.cuda.is_available() or not torch.cuda.is_bf16_supported()):
            raise ValueError("BF16 GPU unavailable")
        dtype = torch.bfloat16 if device == "gpu" else torch.float32
        model = SentenceTransformer(
            str(path), device="cuda:0" if device == "gpu" else "cpu",
            trust_remote_code=False, local_files_only=True,
            model_kwargs={"torch_dtype": dtype, "use_safetensors": True},
            config_kwargs={"vision_config": None, "audio_config": None},
        )
        model.eval()
        model.max_seq_length = int(profile["context_tokens"])
        torch.set_num_threads(int(os.environ.get("BUTLER_MODEL_CPU_THREADS", "4")))
    emit({"kind": "ready", "vram_mib": ((torch.cuda.memory_reserved() + 1048575) // 1048576) if device == "gpu" else 0})
    for line in sys.stdin:
        if len(line) > 4 * 1024 * 1024:
            emit({"kind": "error", "code": "invalid_input"})
            continue
        try:
            request = json.loads(line)
            if request.get("op") != "embed" or not isinstance(request.get("texts"), list):
                raise ValueError("unsupported operation")
            texts = request["texts"]
            if not texts or len(texts) > int(profile["max_batch"]):
                raise ValueError("invalid batch")
            with contextlib.redirect_stdout(sys.stderr), torch.inference_mode():
                # Reject over-limit text instead of silently truncating evidence.
                for text in texts:
                    if not isinstance(text, str) or len(model.tokenizer(text)["input_ids"]) > model.max_seq_length:
                        raise ValueError("input too long")
                embeddings = model.encode(texts, batch_size=len(texts), show_progress_bar=False, normalize_embeddings=False, convert_to_numpy=True)
            emit({"kind": "embeddings", "embeddings": embeddings.tolist()})
        except torch.OutOfMemoryError:
            emit({"kind": "error", "code": "out_of_memory"})
        except ValueError:
            emit({"kind": "error", "code": "invalid_input"})
        except Exception:
            emit({"kind": "error", "code": "model_unavailable"})
except Exception as error:
    emit({"kind": "error", "code": "out_of_memory" if type(error).__name__ == "OutOfMemoryError" else "model_unavailable"})
    sys.exit(1)
