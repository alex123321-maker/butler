// Procedural SVG renderer retained from the approved calendar. Vue owns data and lifecycle.
import {build} from './vine.js';
    const TAU = Math.PI * 2;
    const clamp = (v, a = 0, b = 1) => Math.max(a, Math.min(b, v));
    const f = n => Number(n.toFixed(2));
    const hash = s => { let h = 2166136261; for (const c of s) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; };
    const random = seed => () => { seed |= 0; seed = seed + 0x6D2B79F5 | 0; let t = Math.imul(seed ^ seed >>> 15, 1 | seed); t ^= t + Math.imul(t ^ t >>> 7, 61 | t); return ((t ^ t >>> 14) >>> 0) / 4294967296; };
    const rgb = hex => hex.replace('#','').match(/../g).map(s => parseInt(s,16));
    const mix = (a, b, t) => { a = rgb(a); b = rgb(b); return '#' + a.map((v, i) => Math.round(v + (b[i] - v) * clamp(t)).toString(16).padStart(2, '0')).join(''); };
    const read = key => { try { return localStorage.getItem(key); } catch { return null; } };
    const write = (key, value) => { try { localStorage.setItem(key, value); } catch { /* Автономный файл также работает без хранилища. */ } };
    const localKey = d => `${d.getFullYear()}-${d.getMonth()+1}-${d.getDate()}`;
    const line = points => points.map((p,i) => `${i?'L':'M'}${f(p[0])} ${f(p[1])}`).join(' ');
    const cubic = (points, t) => {
      const u = 1 - t;
      return [0,1].map(k => u*u*u*points[0][k] + 3*u*u*t*points[1][k] + 3*u*t*t*points[2][k] + t*t*t*points[3][k]);
    };
    const cubicPath = p => `M${p[0].map(f).join(' ')} C${p.slice(1).map(v=>v.map(f).join(' ')).join(' ')}`;
    const radial = (id, color, opacity = .5) => `<radialGradient id="${id}"><stop stop-color="${color}" stop-opacity="${opacity}"/><stop offset="1" stop-color="${color}" stop-opacity="0"/></radialGradient>`;
    const linear = (id, top, bottom, x = 0) => `<linearGradient id="${id}" x1="${x}%" y1="0%" x2="${100-x}%" y2="100%"><stop stop-color="${top}"/><stop offset="1" stop-color="${bottom}"/></linearGradient>`;
    const svg = (defs, content) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 1000" preserveAspectRatio="xMidYMid slice" focusable="false" aria-hidden="true"><defs>${defs}</defs>${content}</svg>`;

    class ThoughtGarden {
      constructor(host, owner, stage) {
        this.host = host;
        this.stage=stage;this.disposed=false;this.listeners=new AbortController();
        this.owner = owner;
        this.mode = 'calm';
        this.context = {events: []};
        this.branches = [];
        this.axes = [];
        this.patternSeed=1;this.pendingPattern=false;
        this.capacity = 24;
        this.frame = 0;
        this.resizeFrame = 0;
        this.active = false;
        this.geometry = null;
        this.root = document.createElement('div');
        this.root.className = 'garden-world';
        this.root.setAttribute('aria-hidden', 'true');
        this.root.innerHTML = `<div class="garden-paper"></div>
          <svg class="garden-canvas" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" focusable="false" preserveAspectRatio="xMidYMin meet">
          <defs>
            <linearGradient id="tg-leaf" x1="10%" y1="0%" x2="70%" y2="100%"><stop stop-color="#bed09b"/><stop offset=".42" stop-color="#97b274"/><stop offset="1" stop-color="#6e9056"/></linearGradient>
            <linearGradient id="tg-leaf-light" x1="0%" y1="0%" x2="65%" y2="100%"><stop stop-color="#e0dfad"/><stop offset=".45" stop-color="#bdcc91"/><stop offset="1" stop-color="#96b275"/></linearGradient>
            <linearGradient id="tg-petal" x1="0%" y1="0%" x2="60%" y2="100%"><stop stop-color="#fffef6"/><stop offset=".68" stop-color="#f2ecd8"/><stop offset="1" stop-color="#d7c79f"/></linearGradient>
            <radialGradient id="tg-bud" cx="32%" cy="25%"><stop stop-color="#e0dfbc"/><stop offset=".65" stop-color="#b6bd92"/><stop offset="1" stop-color="#7f916b"/></radialGradient>
            <radialGradient id="tg-earth"><stop stop-color="#b6ae8f" stop-opacity=".28"/><stop offset=".38" stop-color="#c7c6a9" stop-opacity=".19"/><stop offset=".72" stop-color="#d8dcc4" stop-opacity=".08"/><stop offset="1" stop-color="#e6e9d8" stop-opacity="0"/></radialGradient>
            <radialGradient id="tg-moss"><stop stop-color="#9caa82" stop-opacity=".23"/><stop offset=".48" stop-color="#bec9a5" stop-opacity=".13"/><stop offset="1" stop-color="#dce3ce" stop-opacity="0"/></radialGradient>
            <radialGradient id="tg-soil-cover"><stop stop-color="#d5d4bd"/><stop offset=".22" stop-color="#d5d4bd"/><stop offset=".55" stop-color="#d6d6bf" stop-opacity=".55"/><stop offset="1" stop-color="#dedfcb" stop-opacity="0"/></radialGradient>
            <radialGradient id="tg-soil-shadow"><stop stop-color="#7e8063" stop-opacity=".3"/><stop offset="1" stop-color="#a9ad8d" stop-opacity="0"/></radialGradient>
          </defs>
          <defs><clipPath id="tg-safe" clipPathUnits="userSpaceOnUse"><path class="garden-exclusion" clip-rule="evenodd" fill-rule="evenodd"/></clipPath></defs>
            <g class="garden-ground">
              <ellipse cx="-10" cy="3" rx="112" ry="36" fill="url(#tg-earth)"/>
              <ellipse cx="-47" cy="6" rx="72" ry="25" fill="url(#tg-moss)" transform="rotate(-9 -47 6)"/>
              <ellipse cx="34" cy="0" rx="61" ry="22" fill="url(#tg-moss)" transform="rotate(12 34 0)"/>
              <ellipse cx="7" cy="6" rx="49" ry="19" fill="url(#tg-earth)"/>
            </g>
          <g clip-path="url(#tg-safe)">
            <g class="garden-main"></g><g class="garden-branches"></g>
            <g class="garden-root-cover">
              <ellipse cx="0" cy="3" rx="27" ry="9" fill="url(#tg-soil-cover)"/>
              <ellipse cx="-1" cy="1" rx="10" ry="2.5" fill="url(#tg-soil-shadow)"/>
              <g fill="#9c9d7d" opacity=".22"><ellipse cx="-13" cy="7" rx="1.5" ry=".7"/><ellipse cx="8" cy="10" rx="1.1" ry=".6"/><ellipse cx="20" cy="4" rx="1.8" ry=".6"/><ellipse cx="-26" cy="13" rx="1" ry=".6"/><ellipse cx="29" cy="17" rx="1.3" ry=".6"/></g>
            </g>
          </g></svg>`;
        host.insertBefore(this.root, host.querySelector('.scene-grain'));
        this.svg = this.root.querySelector('svg');
        this.mainLayer = this.root.querySelector('.garden-main');
        this.branchLayer = this.root.querySelector('.garden-branches');
        this.variantButtons = [];
        this.variantButtons.forEach(b => b.addEventListener('click', () => this.chooseMode(b.dataset.plant)));
        this.syncMode();
        this.observer = new ResizeObserver(() => this.queueLayout());
        this.observer.observe(this.stage);
        window.addEventListener('resize', () => this.queueLayout(), {signal:this.listeners.signal});

        document.addEventListener('visibilitychange', () => { if (document.hidden) this.finish(); else this.refreshBlooms(); }, {signal:this.listeners.signal});
        window.addEventListener('focus',()=>this.refreshBlooms(), {signal:this.listeners.signal});

      }
      el(name, attrs = {}) {
        const el = document.createElementNS('http://www.w3.org/2000/svg', name);
        for (const [key, value] of Object.entries(attrs)) el.setAttribute(key, String(value));
        return el;
      }
      smooth(x) { x = clamp(x); return x * x * (3 - 2 * x); }
      record(value = 0) { return {progress:value, from:value, target:value, start:0, duration:0}; }
      sample(record, now) {
        if (!record.duration) return record.progress;
        const t = (now - record.start) / record.duration;
        record.progress = record.from + (record.target - record.from) * this.smooth(t);
        if (t >= 1) { record.progress = record.target; record.duration = 0; }
        return record.progress;
      }
      animate(record, target, now, delay = 0, duration = 1100) {
        this.sample(record, now);
        record.from = record.progress;
        record.target = target;
        record.start = now + delay;
        record.duration = Math.abs(target - record.progress) < .0001 ? 0 : duration;
      }
      staticMotion() { return this.owner.paused || this.owner.motion.matches || document.hidden; }
      setActive(active) {
        this.active = active;
        this.root.classList.toggle('is-visible', active);
        if (!active) this.finish();
      }
      syncMode() {
        this.root.dataset.mode = this.mode;
        this.variantButtons.forEach(b => b.setAttribute('aria-pressed', String(b.dataset.plant === this.mode)));
      }
      chooseMode(mode) {
        if (!['calm', 'rich'].includes(mode) || mode === this.mode) return;
        this.mode = mode;
        write('butler.mind-prototype.garden.mode.v4', mode);
        this.syncMode();
        // Обе версии имеют одну топологию: насыщенность — только в листве и пластике.
        this.layout(true);
      }
      destroy() {this.disposed=true;this.listeners.abort();this.observer.disconnect();cancelAnimationFrame(this.frame);cancelAnimationFrame(this.resizeFrame);clearTimeout(this.bloomTimer);clearTimeout(this.highlightTimer);this.root.remove();}
      queueLayout() {
        if(this.disposed)return;
        cancelAnimationFrame(this.resizeFrame);
        this.resizeFrame = requestAnimationFrame(() => {
          this.resizeFrame = 0;
          if (this.active) this.layout();
        });
      }
      update(state, context) {
        if(this.disposed)return;
        const previousCount=this.context.events.length;
        this.context = context || {events: []};
        const events = this.context.events;
        const shrinking=events.length<previousCount;
        const sampledAt=performance.now();
        for(const axis of this.axes)this.sample(axis.life,sampledAt);
        for(const b of this.branches)this.sample(b.life,sampledAt);
        // A smaller or equally busy day keeps the plant that is already there.
        // Only an increase asks the solver for a new direction.
        if(!this.geometry||events.length>previousCount){this.patternSeed=crypto.getRandomValues(new Uint32Array(1))[0];this.pendingPattern=true;this.root.dataset.pattern=String(this.patternSeed);}
        this.root.style.setProperty('--garden-sheet', mix(state.base, '#f4f2e8', .84));
        this.root.dataset.count = String(events.length);
        this.root.dataset.completed = String(events.filter(e => e.done === true).length);
        // Ни одна задача не отбрасывается. 24 места заранее удерживают композицию
        // при типичном переключении 3 -> 8 -> 20; сверх этого сетка уплотняется.
        this.capacity = Math.max(this.capacity, Math.ceil(events.length / 8) * 8);
        while (this.branches.length < events.length) this.createBranch(this.branches.length);
        this.layout();
        const now = performance.now();
        const targets=this.growth.stageNodes(events.length).map((node,i)=>this.nodeDistance(this.axes[i].metric,node));
        let added = 0;
        const pruneDuration=1150;
        const pruneDelays=new Map();
        for(const b of [...this.branches].reverse()) {
          if(events[b.index]||b.life.progress<=0)continue;
          let delay=0;
          for(const child of this.branches)if(child.slot?.parent===b.index&&pruneDelays.has(child.index))delay=Math.max(delay,pruneDelays.get(child.index)+pruneDuration);
          pruneDelays.set(b.index,delay);
        }
        // Keep supporting wood in place until its last child has retracted.
        const pruneHold=Math.max(0,...[...pruneDelays.values()].map(delay=>delay+pruneDuration));
        for(const [i,axis] of this.axes.entries()) {
          this.sample(axis.life,now);
          // A rapid reversal may happen before surviving tasks have support.
          // Reach their stage even if it is ahead of the current growing tip.
          const target=targets[i];
          // Smoothstep peaks at 1.5x mean speed. Length-based timing caps
          // retraction at 625 px/s, including the middle of a large jump.
          const retractDuration=Math.max(1600,Math.abs(target-axis.life.progress)*2.4);
          const growDuration=Math.max(1900,Math.abs(target-axis.life.progress)*3.2);
          if(axis.life.target!==target)this.animate(axis.life,target,now,shrinking?pruneHold:0,target>axis.life.progress?growDuration:retractDuration);
        }
        for (const b of this.branches) {
          const event = events[b.index];
          const alive = Boolean(event);
          this.sample(b.life,now);
          const target=alive?1:0;
          if (b.alive !== alive || b.life.target!==target || (shrinking&&alive&&b.life.progress<1)) {
            b.alive = alive;
            const parent=this.branches[b.slot?.parent];
            const attachment=parent?.metric?this.arrival(parent,this.nodeDistance(parent.metric,b.slot.parentNode)/parent.metric.length,now):0;
            const axis=this.axes[b.slot?.axis];
            const arrival=this.arrival(axis,axis?this.nodeDistance(axis.metric,b.slot.axisNode):0,now);
            const delay=alive?Math.max(added++*85,attachment,arrival):(pruneDelays.get(b.index)||0);
            this.animate(b.life, target, now, delay, alive ? 1500 : pruneDuration);
          }
          if (event) {
            b.group.dataset.task = String(event.title || '');
            b.group.dataset.time = String(event.time || '');
            b.group.dataset.done = String(event.done === true);
            b.group.dataset.state = event.status || (event.done ? 'completed' : 'scheduled');
          }
          b.highlightUntil = 0;
          this.paintBranch(b);
        }
        this.refreshBlooms();
        if (this.staticMotion()) this.finish(); else this.requestFrame();
      }
      refreshBlooms() {
        clearTimeout(this.bloomTimer);
        if(!this.context.date||!this.active)return;
        const wall=Date.now(),now=performance.now();let next=Infinity,past=0,changed=false;
        for(const b of this.branches){
          const event=this.context.events[b.index];
          if(!b.alive||!event)continue;
          const [hour,minute]=event.time.split(':').map(Number),date=this.context.date;
          const at=new Date(date.getFullYear(),date.getMonth(),date.getDate(),hour,minute).getTime();
          const target=at<=wall?1:0,key=localKey(date)+'|'+event.time+'|'+event.title+'|'+b.index;
          if(target)past++;else next=Math.min(next,at);
          // Changing the selected day must not reuse another event's flower.
          // Only a clock boundary for the same event animates bud -> flower.
          if(b.eventKey!==key){b.bloom=this.record(target);b.eventKey=key;changed=true;}
          else if(b.bloom.target!==target){this.animate(b.bloom,target,now,0,1000);changed=true;}
          b.tipReveal.dataset.organ=target?'flower':'bud';
          b.tipReveal.dataset.eventIndex=String(b.index);
          this.paintBranch(b,now);
        }
        this.root.dataset.pastCount=String(past);
        this.root.dataset.futureCount=String(this.context.events.length-past);
        if(Number.isFinite(next))this.bloomTimer=setTimeout(()=>this.refreshBlooms(),Math.max(10,Math.min(86400000,next-wall+5)));
        if(changed){if(this.staticMotion())this.finish();else this.requestFrame();}
      }
      createBranch(index) {
        const group = this.el('g', {class:'garden-sprout', 'data-sprout':index});
        const stroke=this.el('path',{class:'garden-reveal',fill:'none',stroke:'none'});
        const stems = this.el('g', {class:'garden-stem-tapers'});
        const defs=this.el('defs'),clip=this.el('clipPath',{id:'tg-grow-'+index,clipPathUnits:'userSpaceOnUse'}),reveal=this.el('path');
        clip.append(reveal);defs.append(stroke,clip);stems.setAttribute('clip-path','url(#tg-grow-'+index+')');
        const foliage = this.el('g', {class:'garden-foliage'});
        const tipPosition = this.el('g');
        const tipReveal = this.el('g');
        const bud = this.el('g', {class:'garden-bud'});
        // Бутон смотрит по касательной стебля и не имеет отдельного несостыкованного усика.
        bud.innerHTML = `<path d="M0 0C-2.8-5.4-4.3-10.4 0-15 C4.3-10.4 2.8-5.4 0 0Z" fill="url(#tg-bud)" stroke="#7e9067" stroke-width=".65"/><path d="M0 0Q-5-3-4.1-6.1M0 0Q5-3 4.1-6.1" fill="none" stroke="#84956f" stroke-width=".75"/><path d="M0-2Q-1.1-7.3 0-12.8" fill="none" stroke="#eff0cf" stroke-opacity=".66" stroke-width=".65"/>`;
        const flower = this.el('g', {class:'garden-flower'});
        flower.innerHTML = [0,1,2,3,4].map(i => `<g transform="rotate(${72*i})"><path d="M0 1C-5 0-7-7.5-4-11.7C-1.7-15.3 3.2-13.5 4.2-9.7C5.2-5.2 3-1.2 0 1Z" fill="url(#tg-petal)" stroke="#bdb691" stroke-width=".65"/><path d="M0-.5Q-1.1-4.3-.3-9.7" fill="none" stroke="#dfd3b2" stroke-width=".6"/></g>`).join('') + `<circle r="3.2" fill="#d5bb79" stroke="#a59358" stroke-width=".5"/><circle cx="-.9" cy="-.7" r=".8" fill="#f6e9b6"/><circle cx="1.1" cy=".6" r=".65" fill="#ad9759"/>`;
        tipReveal.append(bud, flower);
        tipPosition.append(tipReveal);
        group.append(defs, stems, foliage, tipPosition);
        this.branchLayer.append(group);
        this.branches.push({index, group, stroke, clip, reveal, stems, foliage, tipPosition, tipReveal, bud, flower, life:this.record(), bloom:this.record(), leaves:[], alive:false, highlightUntil:0, layoutKey:''});
      }
      // Кубические участки с общими касательными, без искусственных петель.
      pathFromPoints(points) {
        let d = `M${points[0].map(f).join(' ')}`;
        for (let i=0; i<points.length-1; i++) {
          const a=points[Math.max(0,i-1)], b=points[i], c=points[i+1], z=points[Math.min(points.length-1,i+2)];
          const p=[b[0]+(c[0]-a[0])/6, b[1]+(c[1]-a[1])/6];
          const q=[c[0]-(z[0]-b[0])/6, c[1]-(z[1]-b[1])/6];
          d+=`C${p.map(f).join(' ')} ${q.map(f).join(' ')} ${c.map(f).join(' ')}`;
        }
        return d;
      }
      // Sample our own cubic curves once, instead of asking SVG to measure the
      // full path hundreds of times. Main stems have dense knots; shoots do not.
      curveMetric(points) {
        const samples=[],lengths=[];let length=0;
        const steps=points.length>10?5:32;
        for(let i=0;i<points.length-1;i++) {
          const a=points[Math.max(0,i-1)],b=points[i],c=points[i+1],z=points[Math.min(points.length-1,i+2)];
          const px=b[0]+(c[0]-a[0])/6,py=b[1]+(c[1]-a[1])/6,qx=c[0]-(z[0]-b[0])/6,qy=c[1]-(z[1]-b[1])/6;
          for(let j=i?1:0;j<=steps;j++) {
            const t=j/steps,u=1-t,p=[u*u*u*b[0]+3*u*u*t*px+3*u*t*t*qx+t*t*t*c[0],u*u*u*b[1]+3*u*u*t*py+3*u*t*t*qy+t*t*t*c[1]];
            if(samples.length){const before=samples.at(-1);length+=Math.hypot(p[0]-before[0],p[1]-before[1]);}
            samples.push(p);lengths.push(length);
          }
        }
        return {samples,lengths,length,steps,knotCount:points.length};
      }
      sampleMetric(metric,t) {
        const at=clamp(t)*metric.length,{samples,lengths}=metric;
        let lo=1,hi=samples.length-1;
        while(lo<hi){const mid=(lo+hi)>>1;if(lengths[mid]<at)lo=mid+1;else hi=mid;}
        const a=samples[lo-1],b=samples[lo],ratio=clamp((at-lengths[lo-1])/(lengths[lo]-lengths[lo-1]||1));
        return {p:[a[0]+(b[0]-a[0])*ratio,a[1]+(b[1]-a[1])*ratio],angle:Math.atan2(b[1]-a[1],b[0]-a[0])*180/Math.PI};
      }
      nodeDistance(metric,node) {
        const i=Math.floor(node),from=metric.lengths[i*metric.steps],to=metric.lengths[Math.min(metric.knotCount-1,i+1)*metric.steps];
        return from+(to-from)*(node-i);
      }
      layout(force=false) {
        const stage=this.stage;
        const stageWidth=stage.clientWidth,h=stage.clientHeight;
        if(!stageWidth||!h)return;
        const css=getComputedStyle(stage),cw=parseFloat(css.getPropertyValue('--active-w'))||540,ch=parseFloat(css.getPropertyValue('--active-h'))||426;
        const w=Math.min(this.root.clientWidth,1600),bottom=stageWidth<=760?24:46,top=h-bottom-ch,gutter=(w-cw)/2;
        if(gutter<16||top<40){this.queueLayout();return;}
        const key=[stageWidth,w,h,cw,ch,this.mode].join('|'),rebuilding=force||this.geometry?.key!==key;
        const extending=!rebuilding&&this.pendingPattern;
        const params={w,h,cw,ch,top,bottom,count:this.capacity};
        let preserved=[];
        if(rebuilding) {
          this.geometry={stageWidth,w,h,cw,ch,top,gutter,bottom,narrow:w<620,key};
          this.lockedNodes=[];
          this.growth=this.chooseGrowth(params);
          this.svg.setAttribute('viewBox','0 0 '+w+' '+h);
          Object.assign(this.svg.style,{width:w+'px',height:h+'px',left:'50%',transform:'translateX(-50%)'});
          // A static vector exclusion is enough: the opaque moving day card
          // covers the plant itself. No shared animated bitmap mask.
          const r=this.growth.obstacle,x=r.left,y=r.top,a=r.right,b=r.bottom,q=18;
          this.svg.querySelector('.garden-exclusion').setAttribute('d',
            'M0 0H'+w+'V'+h+'H0Z M'+(x+q)+' '+y+'H'+(a-q)+'Q'+a+' '+y+' '+a+' '+(y+q)+'V'+(b-q)+'Q'+a+' '+b+' '+(a-q)+' '+b+'H'+(x+q)+'Q'+x+' '+b+' '+x+' '+(b-q)+'V'+(y+q)+'Q'+x+' '+y+' '+(x+q)+' '+y+'Z');
          this.root.dataset.algorithm='persistent-prefix';
          const [rx,ry]=this.growth.axes[0][0];
          // A local patch of soil, independent of screen width. Overlapping
          // transparent washes have no shared silhouette or hard upper edge.
          this.svg.querySelector('.garden-ground').setAttribute('transform',`translate(${rx} ${ry}) scale(${w<620?.48:1} ${w<620?.75:1})`);
          this.svg.querySelector('.garden-root-cover').setAttribute('transform',`translate(${rx} ${ry}) scale(${w<620?.6:1} 1)`);
          this.buildMain();
        } else if(extending) {
          // Lock the visible cubic curve plus its tangent support. Distances in
          // pixels survive a longer hidden tail without moving the visible tip.
          this.lockedNodes=this.axes.map(axis=>{
            let i=0;while(i<159&&axis.metric.lengths[i*axis.metric.steps]<axis.life.progress)i++;
            return Math.min(159,i+3);
          });
          preserved=this.branches.map(b=>b.life.progress>.0001?b.slot:null);
          for(const b of preserved)if(b)this.lockedNodes[b.axis]=Math.max(this.lockedNodes[b.axis],Math.min(159,b.axisNode+3));
          this.growth=this.chooseGrowth({...params,previousAxes:this.axes.map(a=>a.points),lockedNodes:this.lockedNodes,preservedBranches:preserved,stagePlan:this.growth.stagePlan});
          this.buildMain(true,preserved);
        }
        for(const b of this.branches) {
          if(b.index>=this.context.events.length&&b.life.progress<.0001){if(extending||rebuilding)b.layoutKey='';continue;}
          if(extending&&preserved[b.index]){b.slot=this.growth.branches[b.index];continue;}
          if(extending||b.layoutKey!==key)this.layoutBranch(b,key);
        }
        this.pendingPattern=false;
      }
      chooseGrowth(params) {
        this.root.dataset.pattern=String(this.patternSeed);
        return build({...params,seed:this.patternSeed});
      }
      // Vector corridors clip each stem's already-built contour without an
      // offscreen luminance mask shared by the entire plant.
      clipMetric(metric,width) {
        const radius=width/2,stride=metric.samples.length>200?3:4,left=[],right=[],lengths=[];
        for(let i=0;i<metric.samples.length;i+=stride) {
          const p=metric.samples[i],a=metric.samples[Math.max(0,i-1)],b=metric.samples[Math.min(metric.samples.length-1,i+1)],norm=Math.hypot(b[0]-a[0],b[1]-a[1])||1;
          const x=-(b[1]-a[1])/norm*radius,y=(b[0]-a[0])/norm*radius;
          left.push([p[0]+x,p[1]+y].map(f).join(' '));right.push([p[0]-x,p[1]-y].map(f).join(' '));lengths.push(metric.lengths[i]);
        }
        return {metric,radius,left,right,lengths,full:'M'+left.join('L')+'L'+[...right].reverse().join('L')+'Z'};
      }
      paintClip(el,clip,distance) {
        if(el.gardenDistance===distance&&el.gardenClip===clip)return;
        el.gardenDistance=distance;el.gardenClip=clip;
        if(distance<=.0001){this.pathTo(el,'');return;}
        if(distance>=clip.metric.length-.0001){this.pathTo(el,clip.full);return;}
        let i=0;while(i<clip.lengths.length-1&&clip.lengths[i+1]<distance)i++;
        const node=this.sampleMetric(clip.metric,distance/clip.metric.length),angle=node.angle*Math.PI/180,x=-Math.sin(angle)*clip.radius,y=Math.cos(angle)*clip.radius;
        const capLeft=[node.p[0]+x,node.p[1]+y].map(f).join(' '),capRight=[node.p[0]-x,node.p[1]-y].map(f).join(' ');
        this.pathTo(el,'M'+clip.left.slice(0,i+1).join('L')+'L'+capLeft+'L'+capRight+'L'+clip.right.slice(0,i+1).reverse().join('L')+'Z');
      }
      // A stem is one filled contour. Its width changes continuously, without
      // overlapping caps or visible joins between sections.
      ribbonPath(metric,base,tip,indexed=false) {
        const count=indexed?metric.samples.length-1:192;
        const left=[],right=[];
        for(let i=0;i<=count;i++) {
          const t=i/count,node=indexed?{p:metric.samples[i],angle:Math.atan2(metric.samples[Math.min(count,i+1)][1]-metric.samples[Math.max(0,i-1)][1],metric.samples[Math.min(count,i+1)][0]-metric.samples[Math.max(0,i-1)][0])*180/Math.PI}:this.sampleMetric(metric,t),angle=node.angle*Math.PI/180;
          const radius=(tip+(base-tip)*Math.pow(1-t,.85))/2;
          const nx=-Math.sin(angle)*radius,ny=Math.cos(angle)*radius;
          left.push([node.p[0]+nx,node.p[1]+ny]);right.push([node.p[0]-nx,node.p[1]-ny]);
        }
        return 'M'+left.map(p=>p.map(f).join(' ')).join('L')+'L'+right.reverse().map(p=>p.map(f).join(' ')).join('L')+'Z';
      }
      pathTo(el,d) { if(el.getAttribute('d')!==d)el.setAttribute('d',d); }
      moveTo(el,point,angle) { const transform='translate('+point.map(f).join(' ')+') rotate('+f(angle)+')';if(el.getAttribute('transform')!==transform)el.setAttribute('transform',transform); }
      placeLeaf(leaf,parent,opts,fraction,seed,direction,anchor) {
        if(!leaf)leaf=this.makeLeaf(parent,{...(opts||{point:[0,0],angle:0,size:14}),fraction,seed,direction});
        const destination=opts||anchor;
        if(destination){this.moveTo(leaf.position,destination.point,destination.angle);leaf.size.setAttribute('transform','scale('+f(destination.size)+')');leaf.angle=destination.angle;leaf.pixelSize=destination.size;}
        leaf.fraction=fraction;leaf.direction=direction;leaf.placement=opts;
        leaf.position.style.opacity=opts?'1':'0';
        return leaf;
      }
      reserveLeaf(leaf) {
        const p=leaf?.placement;if(!p)return;
        const angle=p.angle*Math.PI/180;
        this.occupied.push({centre:[p.point[0]+Math.cos(angle)*p.size*.60,p.point[1]+Math.sin(angle)*p.size*.60],radius:p.size*.57+2});
      }
      buildMain(reuse=false,preserved=[]) {
        const previous=this.axes||[];
        if(!reuse)this.mainLayer.replaceChildren();
        this.occupied=[];this.axes=[];
        this.reservedTips=this.growth.branches.map(b=>b.points.at(-1));
        if(reuse){
          for(const axis of previous)for(const leaf of axis.leaves)if(leaf.node<=this.lockedNodes[axis.index])this.reserveLeaf(leaf);
          for(const b of this.branches)if(preserved[b.index])for(const leaf of b.leaves)this.reserveLeaf(leaf);
        }
        const mobile=this.geometry.narrow;
        this.axisWidths=mobile?[2.0,1.4,.95]:[3.2,2.0,1.15];
        for(const [index,points] of this.growth.axes.entries()) {
          let axis=reuse?previous[index]:null;
          const width=this.axisWidths[index],d=this.pathFromPoints(points),metric=this.curveMetric(points);
          if(!axis) {
            const group=this.el('g',{'data-axis':index});this.mainLayer.append(group);
            const defs=this.el('defs'),clip=this.el('clipPath',{id:'tg-axis-'+index,clipPathUnits:'userSpaceOnUse'}),reveal=this.el('path');
            clip.append(reveal);defs.append(clip);group.append(defs);
            const spine=this.el('path',{class:'garden-spine',fill:'none',stroke:'none'}),ribbon=this.el('path',{class:'garden-ribbon main-ribbon','data-width':width,'clip-path':'url(#tg-axis-'+index+')'});
            group.append(spine,ribbon);
            const life=this.record(previous[index]?this.nodeDistance(metric,this.growth.stageNodes(this.context.events.length)[index]):0);
            axis={group,spine,reveal,ribbon,life,leaves:[],index};
            axis.tip=this.el('g',{class:'garden-growing-tip'});
            group.append(axis.tip);
          }
          this.axes.push(axis);axis.points=points;axis.metric=metric;
          this.pathTo(axis.spine,d);axis.clipMetric=this.clipMetric(metric,width+3);
          this.pathTo(axis.ribbon,this.ribbonPath(metric,width,width*.88,true));
          const count=Math.ceil((index<2?this.geometry.w+this.geometry.ch*.65:this.geometry.gutter*2+this.geometry.ch)/(mobile?30:44));
          const nodes=[{node:1,size:mobile?12:14},{node:3,size:mobile?15:18}];
          for(let i=0;i<count;i++)nodes.push({node:Math.floor((.055+(i+.13*Math.sin(i*2.4))/count*.92)*159),size:(mobile?23:37)*(1-index*.05)*(.86+(i*7%5)*.06)});
          for(const [i,node] of nodes.entries()) {
            const distance=this.nodeDistance(metric,node.node),fraction=distance/metric.length;
            if(reuse&&axis.leaves[i]&&node.node<=this.lockedNodes[index]){axis.leaves[i].fraction=fraction;axis.leaves[i].distance=distance;continue;}
            const point=points[node.node],a=points[Math.max(0,node.node-1)],b=points[Math.min(159,node.node+1)];
            const angle=Math.atan2(b[1]-a[1],b[0]-a[0])*180/Math.PI,direction=(i+index)%2?1:-1,seed=i*31+index*19+this.patternSeed%97;
            const opts=this.fitLeaf(point,angle+direction*(48+seed%4*10),node.size);
            const leaf=this.placeLeaf(axis.leaves[i],axis.group,opts,fraction,seed,direction,{point,angle,size:node.size});
            leaf.node=node.node;leaf.distance=distance;axis.leaves[i]=leaf;
          }
          this.paintAxis(axis);
        }
      }
      leafAmount(leaf,available,retracting,now,finish=false) {
        const previous=leaf.unfoldAmount||0;
        let amount;
        if(finish||this.staticMotion()) {leaf.opening=null;amount=available;}
        else if(retracting) {leaf.opening=null;amount=Math.min(previous,available);}
        else {
          // Unfold in time after the supporting stem arrives. A fast tip must
          // not flash an entire leaf into view in one or two frames.
          if(available>previous+.00001&&!leaf.opening)leaf.opening={start:now,from:previous};
          const opening=leaf.opening;
          amount=opening?Math.min(available,opening.from+(1-opening.from)*this.smooth((now-opening.start)/800)):Math.min(previous,available);
          if(opening&&now-opening.start>=800)leaf.opening=null;
        }
        leaf.unfoldAmount=amount;
        return amount;
      }
      paintAxis(axis,now=performance.now(),finish=false) {
        const distance=axis.life.progress,progress=clamp(distance/axis.metric.length);
        axis.group.style.visibility=distance<.0001?'hidden':'visible';
        axis.group.dataset.progress=progress.toFixed(4);axis.group.dataset.distance=distance.toFixed(3);
        this.paintClip(axis.reveal,axis.clipMetric,distance);
        for(const leaf of axis.leaves) {
          const previous=leaf.unfoldAmount,available=this.smooth((distance-leaf.distance)/(this.geometry.narrow?12:22));
          const amount=this.leafAmount(leaf,available,axis.life.target<distance,now,finish);
          if(previous===amount)continue;
          leaf.unfold.setAttribute('transform','scale('+Math.max(.0001,amount).toFixed(4)+')');
          leaf.unfold.setAttribute('opacity',amount.toFixed(4));
        }
        const node=this.sampleMetric(axis.metric,progress);
        axis.tip.setAttribute('transform','translate('+node.p.map(f).join(' ')+') rotate('+f(node.angle+90)+')');
      }
      arrival(axis,distance,now) {
        const r=axis?.life;
        if(!r||!r.duration||r.target<=r.from||distance<=r.progress)return 0;
        const wanted=clamp((distance-r.from)/(r.target-r.from));let lo=0,hi=1;
        for(let i=0;i<12;i++){const t=(lo+hi)/2;if(this.smooth(t)<wanted)lo=t;else hi=t;}
        return Math.max(0,r.start-now+r.duration*hi);
      }
      fitLeaf(point,angle,size) {
        const minimum=this.geometry.narrow?10:14;
        for(const scale of [1,.88,.75])for(const turn of [0,24,-24,48,-48,78,-78]) {
          const a=angle+turn,s=size*scale,rad=a*Math.PI/180;
          const centre=[point[0]+Math.cos(rad)*s*.60,point[1]+Math.sin(rad)*s*.60],radius=s*.57+2;
          if(s<minimum||!this.growth.free(point,3)||!this.growth.free(centre,radius+2))continue;
          if(this.occupied.some(o=>Math.hypot(o.centre[0]-centre[0],o.centre[1]-centre[1])<o.radius+radius+3))continue;
          if(this.reservedTips.some(p=>Math.hypot(p[0]-centre[0],p[1]-centre[1])<radius+8))continue;
          this.occupied.push({centre,radius});
          return {point,angle:a,size:s};
        }
        return null;
      }
      layoutBranch(b,key) {
        const branch=this.growth.branches[b.index];
        if(!branch){b.group.style.visibility='hidden';return;}
        b.slot=branch;
        const d=this.pathFromPoints(branch.points),metric=this.curveMetric(branch.points);
        this.pathTo(b.stroke,d);
        b.metric=metric;
        const length=metric.length,count=branch.dormant?0:b.index===0?1:Math.min(3,Math.max(2,Math.ceil(length/40)));
        const parent=this.branches[branch.parent];
        const parentWidth=parent?parent.tipWidth+(parent.baseWidth-parent.tipWidth)*Math.pow(1-branch.attachment,.85):this.axisWidths[branch.axis]*(.88+.12*Math.pow(1-(branch.axisT||0),.85));
        const continuation=(branch.attachment||branch.axisT||0)>.9;
        const width=parentWidth*(parent?.66:continuation?1:.74);
        b.baseWidth=width;b.tipWidth=width*.86;
        const ink=parent?parent.ink:['#7c925d','#92a777','#9baf7d'][branch.axis];
        b.ink=ink;b.group.style.setProperty('--branch-ink',ink);
        b.clipMetric=this.clipMetric(metric,width+3);
        if(!b.ribbon){b.ribbon=this.el('path',{class:'garden-ribbon task-ribbon'});b.stems.append(b.ribbon);}
        b.ribbon.dataset.width=String(width);this.pathTo(b.ribbon,this.ribbonPath(metric,width,b.tipWidth));
        for(let i=0;i<count;i++) {
          const fraction=.20+(i+.2)/count*.56;
          const node=this.sampleMetric(metric,fraction),direction=(i+b.index+this.patternSeed)%2?1:-1;
          const size=Math.min(this.geometry.narrow?22:33,21+length*.10)*(1-fraction*.25);
          const opts=this.fitLeaf(node.p,node.angle+direction*(48+(b.index%3)*10),size);
          b.leaves[i]=this.placeLeaf(b.leaves[i],b.foliage,opts,fraction,b.index*7+i,direction,{point:node.p,angle:node.angle,size});
        }
        for(let i=count;i<b.leaves.length;i++)b.leaves[i].position.style.opacity='0';
        const {p,angle}=this.sampleMetric(metric,1);
        this.moveTo(b.tipPosition,p,(branch.markerOnly?branch.markerAngle:angle)+90);
        let tipSize=this.geometry.narrow?.65:.84;
        while(tipSize>.28&&!this.growth.free(p,16*tipSize+2))tipSize-=.08;
        b.tipSize=tipSize;b.layoutKey=key;this.paintBranch(b);
      }
      leafMarkup(seed) {
        const fill=seed%3===0?'tg-leaf-light':'tg-leaf';
        const width=seed%4===0?1.10:.92;
        return `<path d="M0 0Q.07 .01 .16 0" fill="none" stroke="#82946b" stroke-width=".026"/>
          <g transform="scale(1 ${width})"><path d="M.14 0C.06-.19 .20-.37 .40-.35C.68-.33 .87-.10 1.10-.02C.86 .07 .65 .35 .40 .31C.21 .28 .10 .12 .14 0Z" fill="url(#${fill})"/>
          <path d="M.14 0Q.59-.04 1.10-.02C.86 .07 .65 .35 .40 .31C.21 .28 .10 .12 .14 0Z" fill="#eff0cf" opacity=".13"/>
          <path d="M.14 0Q.59-.04 1.07-.02M.36-.015 .47-.23M.59-.025 .73-.15M.36-.015 .49 .21M.62-.025 .76 .12" fill="none" stroke="#587646" stroke-opacity=".30" stroke-width=".014" stroke-linecap="round"/></g>`;
      }
      makeLeaf(parent, opts) {
        const position=this.el('g',{class:'leaf-position',transform:`translate(${opts.point.map(f).join(' ')}) rotate(${f(opts.angle)})`});
        const unfold=this.el('g',{class:'leaf-unfold'});
        const size=this.el('g',{class:'leaf-size',transform:`scale(${f(opts.size)})`});
        // Только этот внутренний слой получает CSS-анимацию. Масштаб не пропадает.
        const sway=this.el('g',{class:'garden-leaf-sway',style:`animation-delay:-${opts.seed%13}s`});
        sway.innerHTML=this.leafMarkup(opts.seed);
        size.append(sway); unfold.append(size); position.append(unfold); parent.append(position);
        return {position,size,unfold,angle:opts.angle,pixelSize:opts.size, fraction:opts.fraction, direction:opts.direction};
      }
      paintBranch(b, now=performance.now(),finish=false) {
        if (!b.slot) return;
        const life=clamp(b.life.progress), done=clamp(b.bloom.progress);
        b.group.style.visibility=life<.0001?'hidden':'visible';
        b.stems.style.visibility=b.slot.markerOnly?'hidden':'inherit';
        b.tipReveal.style.visibility=b.alive?'inherit':'hidden';
        b.group.dataset.progress=life.toFixed(3);
        this.paintClip(b.reveal,b.clipMetric,life*b.clipMetric.metric.length);
        for (const leaf of b.leaves) {
          const a=this.leafAmount(leaf,this.smooth((life-leaf.fraction)/.18),b.life.target<life,now,finish);
          leaf.unfold.setAttribute('transform',`rotate(${f((1-a)*leaf.direction*24)}) scale(${Math.max(.0001,a).toFixed(4)})`);
          leaf.unfold.setAttribute('opacity',a.toFixed(4));
        }
        const revealed=this.smooth((life-.85)/.15);
        b.tipReveal.setAttribute('transform',`scale(${(revealed*b.tipSize).toFixed(4)})`);
        b.tipReveal.setAttribute('opacity',revealed.toFixed(4));
        b.bud.setAttribute('transform',`scale(${Math.max(.001,1-done*.4).toFixed(4)})`);
        b.bud.setAttribute('opacity',(1-done).toFixed(4));
        b.bud.style.visibility=done>=1?'hidden':'inherit';
        b.flower.setAttribute('transform',`rotate(${f((1-done)*-26)}) scale(${Math.max(.001,done).toFixed(4)})`);
        b.flower.setAttribute('opacity',done.toFixed(4));
        b.flower.style.visibility=done<=0?'hidden':'inherit';
        b.group.dataset.highlight=String(b.highlightUntil>now);
      }
      requestFrame() {
        if (this.frame || !this.active) return;
        this.root.dataset.animating='true';
        this.frame=requestAnimationFrame(now=>this.step(now));
      }
      step(now) {
        this.frame=0;
        let moving=false;
        for(const axis of this.axes) {
          if(axis.life.duration||axis.leaves.some(l=>l.opening)){this.sample(axis.life,now);this.paintAxis(axis,now);}
          moving ||= !!axis.life.duration||axis.leaves.some(l=>l.opening);
        }
        for (const b of this.branches) {
          const paint=b.life.duration || b.bloom.duration || b.highlightUntil || b.group.dataset.highlight==='true'||b.leaves.some(l=>l.opening);
          if (b.life.duration) this.sample(b.life,now);
          if (b.bloom.duration) this.sample(b.bloom,now);
          if (paint) this.paintBranch(b,now);
          if (b.highlightUntil && b.highlightUntil<=now) b.highlightUntil=0;
          moving ||= !!(b.life.duration || b.bloom.duration || b.highlightUntil)||b.leaves.some(l=>l.opening);
        }
        if (moving && !this.staticMotion()) this.requestFrame();
        else if (moving) this.finish();
        else this.root.dataset.animating='false';
      }
      finish() {
        cancelAnimationFrame(this.frame); this.frame=0;
        const now=performance.now();
        for(const axis of this.axes){axis.life.progress=axis.life.target;axis.life.duration=0;this.paintAxis(axis,now,true);}
        for (const b of this.branches) {
          b.life.progress=b.life.target; b.life.duration=0;
          b.bloom.progress=b.bloom.target; b.bloom.duration=0;
          this.paintBranch(b,now,true);
        }
        this.root.dataset.animating='false';
      }
      highlight(index) {
        const now=performance.now();
        for (const b of this.branches) {
          b.highlightUntil=b.index===index?now+2300:0;
          this.paintBranch(b,now);
        }
        if (!this.staticMotion()) this.requestFrame();
        else {
          clearTimeout(this.highlightTimer);
          this.highlightTimer=setTimeout(()=>{this.branches.forEach(b=>{b.highlightUntil=0;this.paintBranch(b);});},2300);
        }
      }
    }



export {ThoughtGarden};
