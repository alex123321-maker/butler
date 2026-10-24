"""Bounded, offline NF4 feasibility runner. Not the issue's BF16 comparison."""
import argparse, hashlib, json, os, pathlib, resource, signal, subprocess, threading, time
os.environ.update(HF_HUB_OFFLINE="1",TRANSFORMERS_OFFLINE="1",TOKENIZERS_PARALLELISM="false")
ROOT=pathlib.Path("/work/.cache")
def sha(b):return hashlib.sha256(b).hexdigest()
def validate(value,schema):
    types=schema.get("type",[])
    if isinstance(types,str):types=[types]
    ok=any((t=="null" and value is None) or (t=="object" and type(value)is dict) or (t=="array" and type(value)is list) or (t=="string" and type(value)is str) or (t=="boolean" and type(value)is bool) for t in types)
    if not ok:return False
    if "enum"in schema and value not in schema["enum"]:return False
    if type(value)is dict:
        props=schema.get("properties",{})
        if not set(schema.get("required",[]))<=set(value):return False
        if schema.get("additionalProperties") is False and set(value)-set(props):return False
        return all(validate(v,props[k]) for k,v in value.items() if k in props)
    if type(value)is list:return all(validate(v,schema["items"]) for v in value)
    return True
def unique_object(pairs):
    out={}
    for key,value in pairs:
        if key in out:raise ValueError("duplicate_json_key")
        out[key]=value
    return out
def gpu_pids():
    p=subprocess.run(["nvidia-smi","--query-compute-apps=pid","--format=csv,noheader,nounits"],capture_output=True,text=True,timeout=5)
    if p.returncode:raise RuntimeError("gpu_probe")
    return set(p.stdout.strip().splitlines()) if p.stdout.strip() else set()
def main():
    p=argparse.ArgumentParser();p.add_argument("--limit",type=int,default=12);p.add_argument("--name",required=True);p.add_argument("--prompt-version",choices=["v1","ru-v2","ru-v3"],default="v1");p.add_argument("--offset",type=int,default=0);p.add_argument("--single-frame",action="store_true");args=p.parse_args()
    if not 1<=args.limit<=60 or not 0<=args.offset<60 or args.offset+args.limit>60:raise ValueError("budget")
    root=ROOT/"research/inventory-01"
    selected=json.loads((root/"pilot.json").read_text())["entries"]
    # Round robin across packs to cover six packs in the first twelve attempts.
    selected=sorted(selected,key=lambda r:(int(r["id"].split("-")[1])%10,r["pack"]))[args.offset:args.offset+args.limit]
    dest=root/args.name
    dest.mkdir(mode=0o700)
    prompt_path=pathlib.Path("/work/experiments/emoji140")/("prompt.txt" if args.prompt_version=="v1" else "prompt-"+args.prompt_version+".txt")
    prompt=prompt_path.read_text()
    schema=json.loads(pathlib.Path("/work/experiments/emoji140/caption.schema.json").read_text())
    prep=json.loads((ROOT/"preparation.json").read_text())
    if prep["status"]!="ready":raise RuntimeError("weights_not_ready")
    if gpu_pids():raise RuntimeError("gpu_busy")
    import torch
    from transformers import AutoProcessor, Qwen3VLForConditionalGeneration, BitsAndBytesConfig
    from transformers.generation.streamers import BaseStreamer
    class TimingStreamer(BaseStreamer):
        def __init__(self):self.calls=0;self.first=None;self.start=time.monotonic()
        def put(self,value):
            self.calls+=1
            if self.calls==2:self.first=time.monotonic()-self.start
        def end(self):pass
    torch.manual_seed(0);torch.set_num_threads(2)
    torch.cuda.set_per_process_memory_fraction(.82,0)
    config={"prompt_version":args.prompt_version,"prompt_position":"after_images" if args.prompt_version=="ru-v3" else "before_images","model":prep["models"][0],"precision":"NF4 double-quant; BF16 compute/nonquantized modules","batch":1,"max_output_tokens":512,"sample":False,"seed":0,"frames":"single-middle" if args.single_frame else "ordered-eight","resolution":[224,224],"prompt_sha256":sha(prompt.encode()),"schema_sha256":sha(json.dumps(schema,sort_keys=True).encode()),"pilot_sha256":sha((root/"pilot.json").read_bytes()),"packages":prep["packages"],"planned":len(selected),"ids":[r["id"]for r in selected],"torch":torch.__version__,"gpu":torch.cuda.get_device_name(0),"gpu_fraction":.82}
    (dest/"config.json").write_text(json.dumps(config,indent=2))
    (dest/"load-attempt.json").write_text(json.dumps({"status":"started","at":time.time(),"max_output_tokens":512,"planned":len(selected)}))
    started=time.monotonic();load=time.monotonic()
    model=Qwen3VLForConditionalGeneration.from_pretrained(prep["model_path"],local_files_only=True,trust_remote_code=False,quantization_config=BitsAndBytesConfig(load_in_4bit=True,bnb_4bit_quant_type="nf4",bnb_4bit_use_double_quant=True,bnb_4bit_compute_dtype=torch.bfloat16),dtype=torch.bfloat16,device_map={"":0},attn_implementation="sdpa")
    processor=AutoProcessor.from_pretrained(prep["model_path"],local_files_only=True,trust_remote_code=False)
    model.eval();load_seconds=time.monotonic()-load
    own_pids=gpu_pids()
    if len(own_pids)!=1:raise RuntimeError("gpu_contention")
    (dest/"load.json").write_text(json.dumps({"load_seconds":load_seconds,"gpu_allocated_bytes":torch.cuda.memory_allocated(),"gpu_reserved_bytes":torch.cuda.memory_reserved(),"peak_gpu_allocated_bytes":torch.cuda.max_memory_allocated(),"peak_gpu_reserved_bytes":torch.cuda.max_memory_reserved(),"rss_peak_kib":resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,"processor":processor.image_processor.to_dict()},indent=2))
    print(json.dumps({"loaded":True,"seconds":load_seconds,"allocated":torch.cuda.memory_allocated()}),flush=True)
    stop=threading.Event();contention=threading.Event()
    def monitor():
        while not stop.wait(2):
            try:
                if gpu_pids()-own_pids:contention.set()
            except Exception:contention.set()
    thread=threading.Thread(target=monitor,daemon=True);thread.start()
    def deadline(signum,frame):raise TimeoutError("attempt_deadline")
    signal.signal(signal.SIGALRM,deadline)
    try:
        for ordinal,row in enumerate(selected):
            if contention.is_set():raise RuntimeError("gpu_contention")
            folder=root/"frames"/row["sha256"];meta=json.loads((folder/"frames.json").read_text())
            frames=list(enumerate(meta["frames"]))
            if args.single_frame:frames=[frames[len(frames)//2]]
            content=[] if args.prompt_version=="ru-v3" else [{"type":"text","text":prompt}]
            hashes=[]
            for i,frame in frames:
                path=folder/(str(i)+"-light.png")
                hashes.append(sha(path.read_bytes()))
                content.extend([{"type":"text","text":("Кадр: %.6f секунд." if args.prompt_version=="ru-v3" else "Frame at %.6f seconds.")%frame["timestamp"]},{"type":"image","image":str(path)}])
            if args.prompt_version=="ru-v3":content.append({"type":"text","text":prompt})
            record={"id":row["id"],"source_sha256":row["sha256"],"ordinal":ordinal,"status":"started","frame_sha256":hashes,"image_count":len(frames),"started_at":time.time(),"first_token_seconds":None}
            recordfile=dest/(row["id"]+".json")
            recordfile.write_text(json.dumps(record,indent=2))
            t=time.monotonic();torch.cuda.reset_peak_memory_stats();signal.alarm(120)
            try:
                inputs=processor.apply_chat_template([{"role":"user","content":content}],tokenize=True,add_generation_prompt=True,return_dict=True,return_tensors="pt").to(model.device)
                input_tokens=inputs["input_ids"].shape[-1]
                if input_tokens>4096:raise ValueError("input_bound")
                record["input_tokens"]=input_tokens
                record["visual_tokens"]=int((inputs["input_ids"]==model.config.image_token_id).sum())
                record["image_grid_thw"]=inputs.get("image_grid_thw").tolist() if inputs.get("image_grid_thw")is not None else None
                streamer=TimingStreamer();streamer.start=t
                with torch.inference_mode():out=model.generate(**inputs,max_new_tokens=512,do_sample=False,streamer=streamer)
                record["first_token_seconds"]=streamer.first
                torch.cuda.synchronize()
                ids=out[0,input_tokens:];text=processor.decode(ids,skip_special_tokens=True)
                eos=model.generation_config.eos_token_id
                eos=eos if isinstance(eos,list) else [eos]
                record.update(raw_response=text,output_tokens=len(ids),truncated=len(ids)>=512 and int(ids[-1])not in eos)
                try:
                    value=json.loads(text,object_pairs_hook=unique_object)
                    record["schema_valid"]=validate(value,schema)
                    record["parsed"]=value
                except (ValueError,TypeError):record["schema_valid"]=False
                record["status"]="completed" if record["schema_valid"]and not record["truncated"]else "invalid_first_pass"
                del inputs,out,ids
            except Exception as e:
                record.update(status="failed",error_class=type(e).__name__)
            finally:
                signal.alarm(0)
                record.update(total_seconds=time.monotonic()-t,peak_gpu_allocated_bytes=torch.cuda.max_memory_allocated(),peak_gpu_reserved_bytes=torch.cuda.max_memory_reserved(),rss_peak_kib=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss)
                recordfile.write_text(json.dumps(record,ensure_ascii=False,indent=2))
                print(json.dumps({k:record[k]for k in ["id","status","total_seconds"]}),flush=True)
            if record["status"]=="failed":break
    finally:
        stop.set()
        (dest/"finish.json").write_text(json.dumps({"wall_seconds":time.monotonic()-started,"load_seconds":load_seconds,"gpu_contention":contention.is_set(),"no_automatic_retry":True}))
if __name__=="__main__":main()
