"""Research-only offline tokenizer preflight using the already installed SDK.

Go exports exact complete prompts; this script never receives gold annotations,
generates predictions or changes a prompt. The tokenizer is pinned separately
from the quantized checkpoint. No new Python dependency is introduced.
"""
import hashlib
import json
import pathlib
import urllib.request
from tokenizers import Tokenizer

root = pathlib.Path('/models')
tokenizer_file = root / 'tokenizer.json'
if hashlib.sha256(tokenizer_file.read_bytes()).hexdigest() != '5f9e4d4901a92b997e463c1f46055088b6cca5ca61a6522d1b9f64c4bb81cb42':
    raise ValueError('tokenizer pin mismatch')
tokenizer = Tokenizer.from_file(str(tokenizer_file))
counts = {}
for digest, prompt in json.loads((root / 'preflight-prompts.json').read_text()).items():
    if hashlib.sha256(prompt.encode()).hexdigest() != digest:
        raise ValueError('prompt identity mismatch')
    count = len(tokenizer.encode(prompt, add_special_tokens=False).ids)
    if count < 1 or count > 2032:
        raise ValueError('complete prompt exceeds context')
    counts[digest] = count
(root / 'token-counts.json').write_text(json.dumps(counts, indent=2))
with urllib.request.urlopen('http://decision-qwen:11434/api/tags', timeout=5) as response:
    models = json.load(response)['models']
model = next(m for m in models if m['name'] == 'butler-decision-qwen35-4b:q4-v2')
pins = {
    'ollama_manifest_sha256': model['digest'],
    'model': 'Qwen/Qwen3.5-4B',
    'quantization': 'Q4_K_M',
    'quantized_repository': 'unsloth/Qwen3.5-4B-GGUF',
    'quantized_revision': 'e87f176479d0855a907a41277aca2f8ee7a09523',
    'weight_sha256': '00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4',
    'weight_bytes': 2740937888,
    'tokenizer_revision': '851bf6e806efd8d0a36b00ddf55e13ccb7b8cd0a',
    'tokenizer_sha256': '5f9e4d4901a92b997e463c1f46055088b6cca5ca61a6522d1b9f64c4bb81cb42',
    'runtime': 'Ollama 0.40.2',
    'runtime_image': 'sha256:b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb',
    'runtime_binary_sha256': '8ff456e3f8c08b7fb211857c634d4df8e1a84cacaaf5b9e7b6bbf89e0f9ae8e0',
    'context_tokens': 2048,
    'preflight_count': len(counts),
    'preflight_min_tokens': min(counts.values()),
    'preflight_max_tokens': max(counts.values()),
    'production_runtime_integrated': False,
}
(root / 'qwen-pins.json').write_text(json.dumps(pins, indent=2))
print(json.dumps(pins))
