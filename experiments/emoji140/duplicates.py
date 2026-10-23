"""Potential near-duplicate pairs, never automatically independent examples."""
import json,pathlib
from PIL import Image
root=pathlib.Path("/work/.cache/research/inventory-01")
rows=json.loads((root/"preprocessing.json").read_text())
def dhash(path):
    im=Image.open(path).convert("RGBA")
    bbox=im.getchannel("A").getbbox()
    if bbox:im=im.crop(bbox)
    bg=Image.new("RGBA",im.size,"white");bg.alpha_composite(im)
    pixels=list(bg.convert("L").resize((9,8),Image.Resampling.LANCZOS).getdata())
    return sum((1<<i)for i in range(64)if pixels[(i//8)*9+i%8]>pixels[(i//8)*9+i%8+1])
indexed=[]
for r in rows:
    if r["status"]!="decoded":continue
    index=len(r["frames"])//2
    indexed.append((r["sha256"],dhash(root/"frames"/r["sha256"]/(str(index)+".png"))))
pairs=[]
for i,(a,ha)in enumerate(indexed):
    for b,hb in indexed[i+1:]:
        distance=(ha^hb).bit_count()
        if distance<=4:pairs.append({"a":a,"b":b,"distance":distance,"review":"unreviewed_candidate"})
out={"method":"64-bit horizontal dHash on alpha-cropped middle frame, white background; distance<=4","limitations":"Candidates require visual review; phase/style can cause misses and false positives. No study split is frozen.","pairs":sorted(pairs,key=lambda r:(r["distance"],r["a"],r["b"]))}
(root/"near-duplicates.json").write_text(json.dumps(out,indent=2))
print(json.dumps({"near_duplicate_candidates":len(pairs)}))
