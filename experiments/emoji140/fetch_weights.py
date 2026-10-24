"""One-off bounded range download, pinned public weights and full SHA256 verification.
No automatic retry; incomplete ranges must never be presented as a valid model.
"""
import concurrent.futures,hashlib,json,os,pathlib,time,urllib.request
ROOT=pathlib.Path("/work/.cache")
os.umask(0o077)
proxy=pathlib.Path("/private/telegram.proxy").read_text().strip()
os.environ.update(HTTPS_PROXY=proxy,HTTP_PROXY=proxy,HF_HOME=str(ROOT/"hf"),HF_HUB_DISABLE_XET="1",HF_HUB_DISABLE_PROGRESS_BARS="1",HOME=str(ROOT/"home"))
pins=json.loads(pathlib.Path("/work/experiments/emoji140/pins.json").read_text())
model=pins["models"][0];revision=model["revision"];repo=model["model"]
target=ROOT/"model"/revision
result={"models":pins["models"],"packages":pins["packages"],"model_path":str(target),"started":time.time(),"status":"started","download_mode":"six parallel bounded HTTP ranges through existing proxy","files":[]}
def request(url,headers=None):
 return urllib.request.urlopen(urllib.request.Request(url,headers=headers or {}),timeout=60)
with request("https://huggingface.co/api/models/"+repo+"/tree/"+revision+"?recursive=false&expand=false")as r:tree=json.load(r)
def digest(path):
 h=hashlib.sha256()
 with path.open("rb")as f:
  for b in iter(lambda:f.read(4*1024*1024),b""):h.update(b)
 return h.hexdigest()
try:
 jobs=[];handles=[]
 for meta in tree:
  name=meta["path"]
  if name not in ["model-00001-of-00002.safetensors","model-00002-of-00002.safetensors"]:continue
  expected=meta["lfs"]["oid"];size=meta["size"]
  if not 0<size<6*1024**3:raise ValueError("weight_size")
  dest=target/name
  if dest.exists():
   if digest(dest)!=expected:raise ValueError("existing_integrity")
   result["files"].append({"name":name,"sha256":expected,"bytes":size,"status":"already_verified"});continue
  part=target/(name+".range-partial")
  if part.exists():raise ValueError("prior_range_attempt")
  candidates=list((target/".cache/huggingface/download").glob("*"+expected+"*.incomplete"))
  if len(candidates)>1:raise ValueError("ambiguous_partial")
  start=0
  if candidates:
   start=candidates[0].stat().st_size
   if start>size:raise ValueError("partial_size")
   candidates[0].rename(part)
  else:part.touch()
  fd=os.open(part,os.O_RDWR);handles.append(fd)
  os.ftruncate(fd,size)
  info={"name":name,"sha256":expected,"bytes":size,"reused_prefix":start,"status":"ranges_started"}
  result["files"].append(info)
  chunk=256*1024*1024
  for begin in range(start,size,chunk):
   end=min(begin+chunk,size)-1
   jobs.append((name,fd,begin,end,size))
 def download(job):
  name,fd,begin,end,size=job;t=time.monotonic()
  url="https://huggingface.co/"+repo+"/resolve/"+revision+"/"+name
  expected="bytes %d-%d/%d"%(begin,end,size)
  with request(url,{"Range":"bytes=%d-%d"%(begin,end)})as r:
   if r.status!=206 or r.headers.get("Content-Range")!=expected:raise ValueError("range_contract")
   offset=begin
   while offset<=end:
    b=r.read(min(1024*1024,end-offset+1))
    if not b:raise ValueError("short_range")
    written=os.pwrite(fd,b,offset)
    if written!=len(b):raise ValueError("short_write")
    offset+=written
   if r.read(1):raise ValueError("long_range")
  return {"name":name,"begin":begin,"end":end,"seconds":time.monotonic()-t}
 completed=[]
 (ROOT/"range-download.json").write_text(json.dumps(result,indent=2))
 with concurrent.futures.ThreadPoolExecutor(max_workers=6)as executor:
  for f in concurrent.futures.as_completed([executor.submit(download,j)for j in jobs]):
   completed.append(f.result())
   (ROOT/"ranges-completed.json").write_text(json.dumps(completed,indent=2))
   print(json.dumps({"ranges_completed":len(completed),"ranges_planned":len(jobs)}),flush=True)
 for fd in handles:os.fsync(fd);os.close(fd)
 for info in result["files"]:
  if info["status"]=="already_verified":continue
  part=target/(info["name"]+".range-partial")
  if digest(part)!=info["sha256"]:raise ValueError("weight_integrity")
  part.rename(target/info["name"]);info["status"]="verified"
 from huggingface_hub import snapshot_download
 snapshot_download(repo,revision=revision,local_dir=str(target),token=False,max_workers=2,allow_patterns=["*.json","*.txt","*.jinja"])
 result["status"]="ready"
 result["downloaded_bytes"]=sum(x["bytes"]for x in result["files"])
except Exception as e:
 result["status"]="failed";result["error_class"]=type(e).__name__
finally:
 result["finished"]=time.time();result["download_seconds"]=result["finished"]-result["started"]
 (ROOT/"preparation.json").write_text(json.dumps(result,indent=2))
 print(json.dumps(result),flush=True)
