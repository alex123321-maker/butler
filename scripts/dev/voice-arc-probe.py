"""Fixed SDK/hardware diagnostic only; no audio, models, network or owner data.

Python is required for the official PyTorch XPU SDK. This does not implement a
Butler runtime or grant resource/voice acceptance. The PowerShell parent owns
the finite process deadline and samples its working set.
"""
import contextlib
import json
import sys
import time

class ProbeFailure(Exception):
    pass


def main(progress):
    with contextlib.redirect_stdout(sys.stderr):
        import torch
        if torch.__version__ != "2.10.0+xpu":
            raise ProbeFailure("sdk_identity")
        torch.set_num_threads(2)
        torch.set_num_interop_threads(2)
        progress["stage"] = "xpu_availability"
        if not torch.xpu.is_available():
            raise ProbeFailure("xpu_unavailable")
        properties = torch.xpu.get_device_properties(0)
        torch.xpu.reset_peak_memory_stats(0)
        progress["stage"] = "fixed_tensor"
        started = time.monotonic()
        value = torch.tensor([[1, 2], [3, 4]], dtype=torch.bfloat16, device="xpu:0")
        identity = torch.eye(2, dtype=torch.bfloat16, device="xpu:0")
        result = value @ identity
        torch.xpu.synchronize(0)
        if result.device.type != "xpu" or result.dtype != torch.bfloat16:
            raise ProbeFailure("device_or_dtype")
        if result.float().cpu().tolist() != [[1.0, 2.0], [3.0, 4.0]]:
            raise ProbeFailure("tensor_result")
        return {
            "format": "butler.voice.arc-hardware-probe.v1",
            "result": "FIXED_BF16_TENSOR_PASS_NOT_VOICE_ACCEPTANCE",
            "torch": torch.__version__,
            "device_name": properties.name,
            "reported_device_capacity_bytes": getattr(properties, "total_memory", None),
            "torch_reserved_peak_bytes": torch.xpu.max_memory_reserved(0),
            "tensor_milliseconds": int((time.monotonic() - started) * 1000),
            "model_loads": 0,
            "microphone_capture": False,
            "voice_service_changed": False,
            "resource_admission_qualified": False,
        }


if __name__ == "__main__":
    progress = {"stage": "torch_import"}
    try:
        outcome = main(progress)
    except Exception as exc:
        # No framework exception message or environment material is published.
        reason = exc.args[0] if isinstance(exc, ProbeFailure) else "sdk_execution_error"
        winerror = getattr(exc, "winerror", None)
        if not isinstance(winerror, int):
            winerror = None
        print(json.dumps({"format": "butler.voice.arc-hardware-probe.v1",
                          "result": "FAILED", "error_class": type(exc).__name__,
                          "stage": progress["stage"], "safe_reason": reason,
                          "winerror": winerror,
                          "model_loads": 0, "resource_admission_qualified": False}))
        sys.exit(1)
    print(json.dumps(outcome, ensure_ascii=False, allow_nan=False, sort_keys=True))
