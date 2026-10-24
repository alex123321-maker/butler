"""Separate user-requested 52-item pack; never changes the original corpus."""
import json,pathlib
from PIL import Image,ImageDraw
p=pathlib.Path('/work/experiments/emoji140');private=pathlib.Path('/work/.cache/research/kolobki-01')
m=json.loads((private/'manifest.json').read_text()); rows=sorted(m['entries'],key=lambda r:r['sha256'])
assert len(rows)==52 and all(r['status']=='downloaded' for r in rows)
out=pathlib.Path('/work/.cache/kolobki-sheets');out.mkdir(exist_ok=False)
corpus=[]
for i,r in enumerate(rows):
    corpus.append({'id':'kolobki-%03d'%(i+1),'sha256':r['sha256'],'pack':r['pack'],'custom_emoji_id':r['sticker']['custom_emoji_id'],'unicode':r['sticker']['emoji'],'mime':r['mime']})
for page in range(5):
    subset=corpus[page*12:(page+1)*12];im=Image.new('RGB',(768,len(subset)*108),'#202024');d=ImageDraw.Draw(im)
    for i,r in enumerate(subset):
        folder=private/'frames'/r['sha256'];meta=json.loads((folder/'frames.json').read_text())
        d.text((4,108*i+2),r['id'],fill='white')
        for j in range(len(meta['frames'])):
            tile=Image.open(folder/(str(j)+'-dark.png')).resize((88,88))
            im.paste(tile,(96*j,108*i+18))
    im.save(out/('kolobki-%02d.png'%(page+1)))
with (p/'evidence/kolobki-inventory.json').open('x') as f:json.dump(m,f,ensure_ascii=False,indent=2)
with (p/'evidence/corpus-kolobki.json').open('x') as f:json.dump({'version':'kolobki52-v1','source':'Telegram visible pack tab document ID 5456424794704583026 resolved via getCustomEmojiStickers','scope':'user-requested additional cohort; original450 unchanged','entries':corpus},f,ensure_ascii=False,indent=2)
with (p/'evidence/kolobki-preprocessing.json').open('x') as f:json.dump(json.loads((private/'preprocessing.json').read_text()),f,indent=2)
print(json.dumps({'items':len(corpus),'sheets':5}))
