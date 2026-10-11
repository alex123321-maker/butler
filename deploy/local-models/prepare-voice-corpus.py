"""Pinned public CC-BY-4.0 FLEURS RU samples; no account credentials/inference.

Selection is frozen before model responses: first four <=15s recordings, then
first four additional <=15s recordings containing literal negation or numerals.
Original public archive/TSV stay on storage; conversion is explicit test-only.
"""
import csv, hashlib, io, json, pathlib, re, tarfile, urllib.request
from huggingface_hub import hf_hub_download
import numpy as np
import soundfile as sf

root=pathlib.Path('/data/voice-corpus');root.mkdir(exist_ok=True)
revision='70bb2e84b976b7e960aa89f1c648e09c59f894dd'
repo='google/fleurs'
tsv=hf_hub_download(repo_id=repo,repo_type='dataset',revision=revision,filename='data/ru_ru/test.tsv',token=False)
rows=list(csv.reader(pathlib.Path(tsv).read_text().splitlines(),delimiter='\t'))
assert len(rows)>100 and len(rows[0])>=7
# Dataset schema: id, filename, raw transcription, normalized transcription,
# phonetic transcription, sample count, gender. Selection does not depend on any generated answer.
short=[r for r in rows if len(r)>=7 and r[5].isdigit() and 0<int(r[5])<=240000]
selected=short[:4]
for r in short:
    if r in selected:continue
    if re.search(r'\bне\b|\d',r[2].lower()):selected.append(r)
    if len(selected)==8:break
assert len(selected)==8
archive=hf_hub_download(repo_id=repo,repo_type='dataset',revision=revision,filename='data/ru_ru/audio/test.tar.gz',token=False)
chosen={r[1]:r for r in selected};found=[]
with tarfile.open(archive,'r:gz') as tar:
    for member in tar:
        name=pathlib.PurePosixPath(member.name).name
        if name not in chosen or not member.isfile():continue
        assert member.size<10*1024*1024
        raw=tar.extractfile(member).read();samples,rate=sf.read(io.BytesIO(raw),dtype='float32')
        assert rate==16000 and samples.ndim==1 and len(samples)<=240000 and np.isfinite(samples).all()
        output=io.BytesIO();sf.write(output,samples,16000,format='WAV',subtype='PCM_16')
        # soundfile writes a 44-byte canonical PCM header for this fixed format.
        wave=output.getvalue();assert wave[:4]==b'RIFF' and wave[36:40]==b'data' and len(wave)==44+len(samples)*2
        (root/name).write_bytes(wave)
        r=chosen[name]
        found.append({'id':r[0]+'-'+name,'file':name,'reference':r[2],
                      'samples':len(samples),'original_sha256':hashlib.sha256(raw).hexdigest(),
                      'pcm16_sha256':hashlib.sha256(wave).hexdigest()})
assert len(found)==8
found.sort(key=lambda s:list(chosen).index(s['file']))
def digest(path):
    h=hashlib.sha256()
    with open(path,'rb') as stream:
        while block:=stream.read(1024*1024):h.update(block)
    return h.hexdigest()
(root/'corpus.json').write_text(json.dumps({'dataset':repo,'revision':revision,'license':'CC-BY-4.0',
 'attribution':'Google FLEURS; Conneau et al. 2022; https://huggingface.co/datasets/google/fleurs',
 'selection':'first_four_short_then_first_four_additional_negation_or_number_before_inference',
 'tsv_sha256':digest(tsv),'archive_sha256':digest(archive),'samples':found},ensure_ascii=False,indent=2))
print('Prepared 8 frozen public Russian human recordings; CC-BY-4.0; no inference.')
