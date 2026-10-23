"""Offline bounded media preprocessing for research, never a production worker."""
import argparse, gzip, hashlib, io, json, math, os, pathlib, re, resource, subprocess, sys, time
ROOT=pathlib.Path("/work/.cache")
def digest(b): return hashlib.sha256(b).hexdigest()
def child(source, mime, target):
    resource.setrlimit(resource.RLIMIT_CPU,(20,20))
    resource.setrlimit(resource.RLIMIT_AS,(1024**3,1024**3))
    from PIL import Image, ImageChops, ImageStat
    data=pathlib.Path(source).read_bytes()
    target=pathlib.Path(target);target.mkdir(mode=0o700)
    duration=None; timestamps=[0.0]; frames=[]; indices=[]
    if mime=="application/x-tgsticker":
        from rlottie_python import LottieAnimation
        with gzip.GzipFile(fileobj=io.BytesIO(data)) as f: decoded=f.read(2*1024*1024+1)
        if len(decoded)>2*1024*1024: raise ValueError("tgs_expansion")
        d=json.loads(decoded)
        if any(a.get("p") or a.get("u") for a in d.get("assets",[])): raise ValueError("external_assets")
        anim=LottieAnimation.from_data(decoded.decode("utf-8"))
        total=anim.lottie_animation_get_totalframe(); rate=anim.lottie_animation_get_framerate()
        if not (1<=total<=1800 and 0<rate<=120): raise ValueError("timeline")
        duration=total/rate
        indices=[min(total-1,int(k*total/8)) for k in range(8)]
        timestamps=[i/rate for i in indices]
        frames=[anim.render_pillow_frame(frame_num=i,width=224,height=224).convert("RGBA") for i in indices]
    elif mime=="video/webm":
        import imageio_ffmpeg
        exe=imageio_ffmpeg.get_ffmpeg_exe()
        probe=subprocess.run([exe,"-hide_banner","-i",source],capture_output=True,timeout=5).stderr.decode("utf-8","replace")
        match=re.search(r"Duration: (\d+):(\d+):(\d+\.\d+)",probe)
        if not match: raise ValueError("duration")
        duration=int(match[1])*3600+int(match[2])*60+float(match[3])
        if not 0<duration<=30: raise ValueError("duration")
        timestamps=[duration*k/8 for k in range(8)]
        for ts in timestamps:
            cmd=[exe,"-v","error","-threads","1","-c:v","libvpx-vp9","-i",source,"-ss",str(ts),"-frames:v","1","-vf","scale=224:224:flags=lanczos","-threads","1","-f","rawvideo","-pix_fmt","rgba","pipe:1"]
            p=subprocess.run(cmd,capture_output=True,timeout=5)
            if p.returncode or len(p.stdout)!=224*224*4: raise ValueError("video_decode")
            frames.append(Image.frombytes("RGBA",(224,224),p.stdout))
    else:
        with Image.open(io.BytesIO(data)) as im:
            if im.width>512 or im.height>512: raise ValueError("dimensions")
            frames=[im.convert("RGBA").resize((224,224),Image.Resampling.LANCZOS)]
    if not frames or not any(f.getchannel("A").getbbox() for f in frames): raise ValueError("empty_render")
    rows=[]; light=[]; dark=[]
    for i,(ts,im) in enumerate(zip(timestamps,frames)):
        alpha=im.getchannel("A")
        occupied=sum(n for v,n in enumerate(alpha.histogram()) if v>0)/(im.width*im.height)
        # Palette excludes fully transparent pixels.
        colors={}
        for r,g,b,a in im.getdata():
            if a: colors[(r//32*32,g//32*32,b//32*32)]=colors.get((r//32*32,g//32*32,b//32*32),0)+1
        palette=[list(c) for c in sorted(colors,key=colors.get,reverse=True)[:8]]
        im.save(target/(str(i)+".png"))
        white=Image.new("RGBA",im.size,"#f4f4f4");white.alpha_composite(im);white.convert("RGB").save(target/(str(i)+"-light.png"))
        black=Image.new("RGBA",im.size,"#202024");black.alpha_composite(im);black.convert("RGB").save(target/(str(i)+"-dark.png"))
        light.append(white.convert("RGB"));dark.append(black.convert("RGB"))
        rows.append({"timestamp":ts,"file":str(i)+".png","sha256":digest((target/(str(i)+".png")).read_bytes()),"alpha_occupancy":occupied,"bbox":alpha.getbbox(),"palette":palette})
    movement=[sum(ImageStat.Stat(ImageChops.difference(a,b)).mean)/3/255 for a,b in zip(light,light[1:])]
    sheet=Image.new("RGB",(112*len(light),112),"white")
    for i,im in enumerate(light): sheet.paste(im.resize((112,112)),(i*112,0))
    sheet.save(target/"contact.png")
    result={"status":"decoded","version":"rgba224-8frames-v1","source_sha256":digest(data),"mime":mime,"duration_seconds":duration,"frame_indices":indices,"frames":rows,"mean_motion":sum(movement)/len(movement) if movement else None,"backgrounds":["#f4f4f4","#202024"]}
    (target/"frames.json").write_text(json.dumps(result,indent=2))
def main():
    if len(sys.argv)>1 and sys.argv[1]=="--child":
        try: child(*sys.argv[2:]);return
        except Exception as e: print(type(e).__name__);sys.exit(2)
    parser=argparse.ArgumentParser();parser.add_argument("--inventory",required=True);args=parser.parse_args()
    inventory=pathlib.Path(args.inventory);m=json.loads((inventory/"manifest.json").read_text())
    dest=inventory/"frames";dest.mkdir(mode=0o700,exist_ok=True)
    results=[];seen=set()
    for row in m["entries"]:
        sha=row.get("sha256")
        if not sha or sha in seen: continue
        seen.add(sha);target=dest/sha;start=time.time()
        try:
            p=subprocess.run([sys.executable,__file__,"--child",str(inventory/"media"/sha),row["mime"],str(target)],capture_output=True,timeout=25)
            result=json.loads((target/"frames.json").read_text()) if p.returncode==0 else {"status":"decode_failed","returncode":p.returncode}
        except subprocess.TimeoutExpired: result={"status":"decode_timeout"}
        result.update(sha256=sha,decode_seconds=time.time()-start)
        results.append(result)
        (inventory/"preprocessing.json").write_text(json.dumps(results,indent=2))
        if len(results)%25==0: print(json.dumps({"decoded":sum(r["status"]=="decoded" for r in results),"attempts":len(results)}),flush=True)
    print(json.dumps({"decoded":sum(r["status"]=="decoded" for r in results),"attempts":len(results)}),flush=True)
if __name__=="__main__": main()
