"""Freeze the available corpus and export numbered sheets before retrieval."""
import hashlib,json,pathlib
from PIL import Image,ImageDraw
base=pathlib.Path('/work/experiments/emoji140')
private=pathlib.Path('/work/.cache/research/inventory-01')
known={}
for name in ['ai-annotations.json','ai-food-inventory.json']:
    for row in json.loads((base/name).read_text())['entries']:
        known[row['sha256']]=row['id']
manifest=json.loads((base/'evidence/inventory.json').read_text())
rows=sorted([r for r in manifest['entries'] if r['status']=='downloaded'],key=lambda r:r['sha256'])
rest=[]
for row in rows:
    if row['sha256'] not in known:
        known[row['sha256']]='rest-%03d'%(len(rest)+1)
        rest.append(row)
corpus=[{'id':known[r['sha256']],'sha256':r['sha256'],'pack':r['pack'],'custom_emoji_id':r['sticker']['custom_emoji_id'],'unicode':r['sticker']['emoji'],'mime':r['mime']}for r in rows]
(base/'evidence/corpus-450.json').write_text(json.dumps({'version':'corpus450-v1','entries':corpus},ensure_ascii=False,indent=2))
dest=private/'followup-sheets';dest.mkdir(exist_ok=False)
for page in range((len(rest)+11)//12):
    subset=rest[page*12:(page+1)*12]
    sheet=Image.new('RGB',(768,108*len(subset)),'#e8e8e8');draw=ImageDraw.Draw(sheet)
    for i,row in enumerate(subset):
        folder=private/'frames'/row['sha256']; meta=json.loads((folder/'frames.json').read_text())
        draw.text((4,i*108+2),known[row['sha256']],fill='black')
        for j in range(len(meta['frames'])):
            im=Image.open(folder/(str(j)+'-light.png')).resize((88,88))
            sheet.paste(im,(j*96,i*108+18))
    sheet.save(dest/('rest-%02d.png'%(page+1)))
print(json.dumps({'remaining':len(rest),'total':len(corpus),'sheets':(len(rest)+11)//12}))
