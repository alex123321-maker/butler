"""Isolated research dependency preparation; no product runtime mutation."""
import json, os, pathlib, shutil, subprocess, time, urllib.request
ROOT = pathlib.Path("/work/.cache")
os.umask(0o077)
proxy = pathlib.Path("/private/telegram.proxy").read_text().strip()
os.environ.update(HTTPS_PROXY=proxy, HTTP_PROXY=proxy, HF_HOME=str(ROOT/"hf"), HF_HUB_DISABLE_XET="1", HF_HUB_DISABLE_PROGRESS_BARS="1", PIP_CACHE_DIR=str(ROOT/"pip"), HOME=str(ROOT/"home"))
def get(url):
    with urllib.request.urlopen(url, timeout=60) as r:
        return json.load(r)
result={"started":time.time(),"models":[],"packages":[]}
try:
    pins=json.loads(pathlib.Path("/work/experiments/emoji140/pins.json").read_text())
    result["models"]=pins["models"]
    result["packages"]=pins["packages"]
    for package in result["packages"]:
        name,version=package["name"],package["version"]
        p=subprocess.run(["/data/venv/bin/python","-m","pip","install","--only-binary=:all:","--no-deps","--target",str(ROOT/"python"),name+"=="+version],stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
        if p.returncode: raise RuntimeError("dependency_install")
    (ROOT/"model-pins.json").write_text(json.dumps(result,indent=2))
    print(json.dumps(result),flush=True)
    if shutil.disk_usage(ROOT).free < 20*1024**3: raise RuntimeError("disk_headroom")
    from huggingface_hub import snapshot_download
    m=result["models"][0]
    target=ROOT/"model"/m["revision"]
    start=time.time()
    snapshot_download(m["model"],revision=m["revision"],local_dir=str(target),token=False,max_workers=2,allow_patterns=["*.json","*.safetensors","*.txt","*.jinja"])
    result["download_seconds"]=time.time()-start
    result["model_path"]=str(target)
    result["downloaded_bytes"]=sum(p.stat().st_size for p in target.rglob("*") if p.is_file())
    result["status"]="ready"
except Exception as e:
    result["status"]="failed"
    result["error_class"]=type(e).__name__
finally:
    result["finished"]=time.time()
    (ROOT/"preparation.json").write_text(json.dumps(result,indent=2))
    print(json.dumps(result),flush=True)
