"""Explicit public evaluation preparation; no inference, credentials or imports of model code."""
import hashlib
import json
import pathlib
import sys
from huggingface_hub import snapshot_download

root = pathlib.Path(sys.argv[1])
profiles = [
    ('fastino/GLiNER2.5-multi-Decide', 'e173bca1f0c4217d7a55b3b0c705d5ece8a0218d',
     'torch2.10.0+cu128/transformers5.19.0/gliner2.1.0',
     '9efe0f88c99f2aa794452e9559dc60e98d60d9fa2bf1b60cf2710411b6da5b4e'),
    ('LiquidAI/d1-omni-600M', '02b55d7076f15129e59ab3f94783f32c4b088674',
     'torch2.10.0+cu128/transformers5.19.0/d1code02b55d7',
     '0713bb05270c2685ad106522f4092bceeeb3a93cf79b401f399a712296c911e1'),
]
reviewed = {
 'audio.py':'c5ff09d52d6a079e79d0505c89dbd0582ba2bc9abf2b26595d8d5b1d0c347bdf',
 'encoder.py':'5f2e20319ee42a7367febf06c7653d7b3f9b28ef99a64cb9ec7739563dcfcffe',
 'modeling_d1.py':'2b71a5c909ec5d3aa6307378a41a0fd24af1e17d5c1c1f8d510582147051a686',
 'prompt.py':'a6b29a55ec8345f1fc1fdbfcc4b64d80d473dc4316f095a62c51cb6cce1194cf',
 'vision.py':'43ad71029f82e20fd26b97574627773d47639d7a3742c8b04fe5bf55f41c4c28',
 'config.json':'dfccae241822f81752426629c88c8b8d7efdd03dd19418b6645c604fad43d619',
 'tokenizer_config.json':'03efc74b752b899b6292ff7e607738be59e6f206fec9aca32bbc05e9184090fc',
}
for model,revision,runtime,weight_hash in profiles:
    path = root/revision
    patterns = ['*.json','*.safetensors','*.model','LICENSE','README.md','encoder_config/*']
    if model.startswith('LiquidAI/'):
        patterns += list(reviewed)
    snapshot_download(repo_id=model, revision=revision, local_dir=str(path), token=False,
                      allow_patterns=patterns, max_workers=2)
    weight = hashlib.sha256()
    with (path/'model.safetensors').open('rb') as stream:
        while block := stream.read(1024*1024):
            weight.update(block)
    if weight.hexdigest()!=weight_hash:
        raise ValueError('pinned checkpoint digest mismatch')
    if model.startswith('LiquidAI/'):
        for name,digest in reviewed.items():
            if hashlib.sha256((path/name).read_bytes()).hexdigest()!=digest:
                raise ValueError('reviewed model code mismatch')
    (path/'butler-model.json').write_text(json.dumps({'model':model,'revision':revision,
        'runtime_version':runtime}),encoding='utf-8')
    print('Pinned public decision checkpoint prepared:',model,revision)
