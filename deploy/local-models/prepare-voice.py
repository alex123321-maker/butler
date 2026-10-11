"""Public, pinned preparation only; weights/libraries stay on the storage bind.

No account token, model code import, model inference or CUDA dependency download.
The minimal reviewed offline inference import paths omit the SDK demos, vLLM,
Gradio/Flask, voice cloning, forced-aligner weights and optional audio extras.
"""
import hashlib
import json
import pathlib
import subprocess
import sys
import urllib.request
from huggingface_hub import snapshot_download

data = pathlib.Path(sys.argv[1])
lock = json.loads(pathlib.Path(sys.argv[2]).read_text())
cache = data/'voice-wheels'
cache.mkdir(exist_ok=True)
for lane, items in lock.items():
    destination = data/'voice'/lane
    destination.mkdir(parents=True, exist_ok=True)
    wheels = []
    for item in items:
        wheel = cache/item['filename']
        if not wheel.exists():
            with urllib.request.urlopen(item['url'],timeout=30) as response, wheel.open('wb') as output:
                while block := response.read(1024*1024):
                    output.write(block)
        if wheel.stat().st_size!=item['size'] or hashlib.sha256(wheel.read_bytes()).hexdigest()!=item['sha256']:
            raise ValueError('pinned wheel mismatch')
        wheels.append(str(wheel))
    subprocess.run([sys.executable,'-m','pip','install','--upgrade','--no-index','--no-deps','--no-compile','--no-build-isolation',
                    '--target',str(destination),*wheels],check=True,stdout=subprocess.DEVNULL)
    print('Pinned offline library overlay prepared:',lane,flush=True)

models = [
 ('Qwen/Qwen3-ASR-0.6B','5eb144179a02acc5e5ba31e748d22b0cf3e303b0',
  'torch2.10.0+cu128/transformers4.57.6/qwen-asr0.0.6',
  {'model.safetensors':'79d6cbd4c98c7bbffe9db2edac07f56cd6637d0d5944b27f6c2b8353840323ea'}),
 ('Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice','85e237c12c027371202489a0ec509ded67b5e4b5',
  'torch2.10.0+cu128/transformers4.57.3/qwen-tts0.1.1',
  {'model.safetensors':'bc3c7e785eb961179c25450d1acff03f839e0002f2f3a5aeb67b5735c0fa2adb',
   'speech_tokenizer/model.safetensors':'836b7b357f5ea43e889936a3709af68dfe3751881acefe4ecf0dbd30ba571258'}),
]
for model,revision,runtime,hashes in models:
    path=data/'models'/revision
    snapshot_download(repo_id=model,revision=revision,local_dir=str(path),token=False,
                      allow_patterns=['*.json','*.safetensors','*.txt','*.model','LICENSE','README.md','speech_tokenizer/*'],max_workers=2)
    for name,digest in hashes.items():
        h=hashlib.sha256()
        with (path/name).open('rb') as stream:
            while block:=stream.read(1024*1024): h.update(block)
        if h.hexdigest()!=digest: raise ValueError('pinned voice checkpoint mismatch')
    (path/'butler-model.json').write_text(json.dumps({'model':model,'revision':revision,'runtime_version':runtime}))
    print('Pinned public voice checkpoint prepared:',model,revision,flush=True)

revision='5cd7945676eb32225748052e2e6a0580e4686a08'
path=data/'models'/revision
path.mkdir(exist_ok=True)
file=data/'voice'/'vad'/'silero_vad'/'data'/'silero_vad.jit'
raw=file.read_bytes()
# Match the packaged tensor to the exact primary-source Git blob, not a mutable
# release name. The file is loaded only after this preparation identity check.
url=f'https://api.github.com/repos/snakers4/silero-vad/contents/src/silero_vad/data/silero_vad.jit?ref={revision}'
with urllib.request.urlopen(url,timeout=30) as response: blob=json.load(response)
git_hash=hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest()
if git_hash!=blob['sha']: raise ValueError('Silero release/Git checkpoint mismatch')
(path/'silero_vad.jit').write_bytes(raw)
(path/'butler-model.json').write_text(json.dumps({'model':'snakers4/silero-vad','revision':revision,
  'runtime_version':'torch2.10.0+cu128/silero-vad6.2.3','weight_sha256':hashlib.sha256(raw).hexdigest()}))
print('Pinned primary Silero checkpoint prepared:',revision,flush=True)
