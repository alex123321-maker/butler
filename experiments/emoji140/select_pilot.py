"""Select a development-only feasibility pilot before reading model outputs."""
import hashlib,json,pathlib
from PIL import Image,ImageDraw
root=pathlib.Path("/work/.cache/research/inventory-01")
m=json.loads((root/"manifest.json").read_text())
available={r["sha256"] for r in json.loads((root/"preprocessing.json").read_text()) if r["status"]=="decoded"}
packs=[p["name"] for p in m["packs"] if p["status"]=="inventoried" and p["name"]!="Strawberry_hot_by_fStikBot"]
selected=[];seen=set()
for pack in packs:
    rows=sorted((r for r in m["entries"] if r["pack"]==pack and r.get("sha256") in available),key=lambda r:hashlib.sha256(r["sticker"]["custom_emoji_id"].encode()).hexdigest())
    count=0
    for row in rows:
        if row["sha256"] in seen:continue
        seen.add(row["sha256"]);count+=1
        selected.append({"id":"pilot-%03d"%(len(selected)+1),"sha256":row["sha256"],"pack":pack,"custom_emoji_id":row["sticker"]["custom_emoji_id"]})
        if count==10:break
pilot={"version":"feasibility-pilot-v1","scope":"development only; no locked test","excluded_pack":"Strawberry_hot_by_fStikBot","entries":selected}
dest=root/"pilot.json"
with dest.open("x") as f:json.dump(pilot,f,indent=2)
for page in range((len(selected)+9)//10):
    rows=selected[page*10:(page+1)*10]
    sheet=Image.new("RGB",(800,120*len(rows)),"#e8e8e8");draw=ImageDraw.Draw(sheet)
    for i,row in enumerate(rows):
        folder=root/"frames"/row["sha256"];meta=json.loads((folder/"frames.json").read_text())
        draw.text((5,i*120+4),row["id"],fill="black")
        for j in range(len(meta["frames"])):
            im=Image.open(folder/(str(j)+"-light.png")).resize((96,96))
            sheet.paste(im,(j*100,i*120+22))
    sheet.save(root/("pilot-sheet-%d.png"%(page+1)))
print(json.dumps({"pilot_elements":len(selected),"sheets":(len(selected)+9)//10}))
