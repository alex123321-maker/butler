"""Subordinate fixed offline tensor worker. Go owns validation/lifecycle/budgets.

No URL/path/context, arbitrary language/speaker, reference voice or generation
options are accepted from requests. The pinned official SDK paths consume numpy
arrays or text only. No voice recognition is authentication or tool permission.
"""
import base64
import hashlib
import io
import json
import pathlib
import sys
import wave
import contextlib
import importlib.metadata

def pcm(wav, seconds):
    raw=base64.b64decode(wav,validate=True)
    if len(raw)>44+16000*2*seconds: raise ValueError('audio bound')
    with wave.open(io.BytesIO(raw),'rb') as reader:
        if reader.getnchannels()!=1 or reader.getsampwidth()!=2 or reader.getframerate()!=16000 or reader.getcomptype()!='NONE':
            raise ValueError('audio format')
        samples=reader.readframes(reader.getnframes())
    import numpy as np
    return np.frombuffer(samples,dtype='<i2').astype(np.float32)/32768.0

def emit(value):
    print(json.dumps(value,ensure_ascii=False,allow_nan=False,separators=(',',':')),flush=True)

def run(init,path):
    profile=init['profile'];name=profile['model'];device=init['device']
    with contextlib.redirect_stdout(sys.stderr):
        import torch
        import numpy as np
        if torch.__version__!='2.10.0+cu128': raise ValueError('torch identity')
        torch.set_num_threads(4)
        target='cuda:0' if device=='gpu' else 'cpu'
        dtype=torch.bfloat16 if device=='gpu' else torch.float32
        if device=='gpu' and (not torch.cuda.is_available() or not torch.cuda.is_bf16_supported()): raise ValueError('BF16 unavailable')
        if name=='Qwen/Qwen3-ASR-0.6B':
            import transformers
            if transformers.__version__!='4.57.6' or importlib.metadata.version('qwen-asr')!='0.0.6':raise ValueError('ASR identity')
            from qwen_asr import Qwen3ASRModel
            model=Qwen3ASRModel.from_pretrained(str(path),dtype=dtype,device_map=target,
                local_files_only=True,trust_remote_code=False,use_safetensors=True,
                attn_implementation='sdpa',max_inference_batch_size=1,max_new_tokens=384)
            model.model.eval()
        elif name=='Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice':
            import transformers
            if transformers.__version__!='4.57.3' or importlib.metadata.version('qwen-tts')!='0.1.1':raise ValueError('TTS identity')
            from qwen_tts import Qwen3TTSModel
            model=Qwen3TTSModel.from_pretrained(str(path),dtype=dtype,device_map=target,
                local_files_only=True,trust_remote_code=False,use_safetensors=True,attn_implementation='sdpa')
            model.model.eval()
        elif name=='snakers4/silero-vad':
            if device!='cpu' or importlib.metadata.version('silero-vad')!='6.2.3':raise ValueError('VAD identity')
            from silero_vad import get_speech_timestamps
            meta=json.loads((path/'butler-model.json').read_text())
            tensor=path/'silero_vad.jit'
            if meta['weight_sha256']!='e1122837f4154c511485fe0b9c64455f7b929c96fbb8d79fbdb336383ebd3720' or hashlib.sha256(tensor.read_bytes()).hexdigest()!=meta['weight_sha256']:raise ValueError('VAD tensor identity')
            model=torch.jit.load(str(tensor),map_location='cpu');model.eval()
        else:raise ValueError('voice model')
    emit({'kind':'ready','vram_mib':(torch.cuda.memory_reserved()+1048575)//1048576 if device=='gpu' else 0})
    for line in sys.stdin:
        try:
            if len(line)>6*1024*1024:raise ValueError('request bound')
            request=json.loads(line)
            with contextlib.redirect_stdout(sys.stderr),torch.inference_mode():
                if request.get('op')=='vad' and name=='snakers4/silero-vad':
                    regions=get_speech_timestamps(torch.from_numpy(pcm(request['wav'],120)),model,
                        sampling_rate=16000,threshold=0.5,min_speech_duration_ms=250,
                        min_silence_duration_ms=300,speech_pad_ms=100,return_seconds=False)
                    if len(regions)>64:raise ValueError('region bound')
                    output={'kind':'vad','regions':[{'start_sample':int(r['start']),'end_sample':int(r['end'])} for r in regions]}
                elif request.get('op')=='asr' and name=='Qwen/Qwen3-ASR-0.6B':
                    complete=[]
                    original=model.model.generate
                    def bounded_generate(*args,**kwargs):
                        if kwargs['input_ids'].shape[-1]>profile['context_tokens']:raise ValueError('encoded context bound')
                        result=original(*args,**kwargs)
                        eos=model.model.generation_config.eos_token_id
                        eos=eos if isinstance(eos,list) else [eos]
                        complete.append(bool(result.sequences.shape[-1] and int(result.sequences[0,-1]) in eos))
                        return result
                    model.model.generate=bounded_generate
                    try:
                        result=model.transcribe(audio=(pcm(request['wav'],15),16000),language='Russian',return_time_stamps=False)
                    finally:model.model.generate=original
                    if len(result)!=1:raise ValueError('ASR result')
                    output={'kind':'asr','text':result[0].text,'complete':len(complete)==1 and complete[0]}
                elif request.get('op')=='tts' and name=='Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice':
                    text=request['text']
                    if not text.strip() or len(text)>200 or len(text.encode())>1024:raise ValueError('TTS text bound')
                    if model.processor.tokenizer(text)['input_ids'].__len__()>profile['context_tokens']:raise ValueError('encoded context bound')
                    waves,rate=model.generate_custom_voice(text=text,language='Russian',speaker='Ryan',instruct='',
                        do_sample=False,subtalker_dosample=False,max_new_tokens=240)
                    if len(waves)!=1 or rate!=24000 or len(waves[0])>rate*20 or not np.isfinite(waves[0]).all():raise ValueError('TTS output bound')
                    samples=(np.clip(waves[0],-1,1)*32767).astype('<i2').tobytes()
                    stream=io.BytesIO()
                    with wave.open(stream,'wb') as out:
                        out.setnchannels(1);out.setsampwidth(2);out.setframerate(rate);out.writeframes(samples)
                    output={'kind':'tts','wav':base64.b64encode(stream.getvalue()).decode('ascii')}
                else:raise ValueError('voice operation')
            emit(output)
        except torch.OutOfMemoryError:emit({'kind':'error','code':'out_of_memory'})
        except (ValueError,KeyError,TypeError):emit({'kind':'error','code':'invalid_input'})
        except Exception:emit({'kind':'error','code':'model_unavailable'})
