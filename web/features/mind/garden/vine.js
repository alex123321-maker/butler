// Approved garden geometry. Pure numeric solver; no application state.
  const distance=(a,b)=>Math.hypot(a[0]-b[0],a[1]-b[1]);
  function build({w,h,cw,ch,top,bottom,count=24,seed=1,pinnedAxes,previousAxes,lockedNodes=[],preservedBranches=[],stagePlan}) {
    let state=seed>>>0;
    const random=()=>{state=(Math.imul(state,1664525)+1013904223)>>>0;return state/4294967296;};
    const left=(w-cw)/2,right=left+cw,mobile=w<620;
    const obstacle={left:left-12,right:right+12,top:top-12,bottom:top+ch+12};
    const freeXY=(x,y,r)=>x>=r+3&&x<=w-r-3&&y>=r+5&&y<=h-bottom-r-8&&
      !(x+r>obstacle.left&&x-r<obstacle.right&&y+r>obstacle.top&&y-r<obstacle.bottom);
    const free=(p,r=0)=>freeXY(p[0],p[1],r);
    const segmentFree=(a,b,r=2)=>{for(let i=0;i<=12;i++)if(!freeXY(a[0]+(b[0]-a[0])*i/12,a[1]+(b[1]-a[1])*i/12,r))return false;return true;};
    const root=[mobile?left*.34:left-Math.min(135,left*.62),top+Math.min(235,ch*(mobile?.28:.57))];
    let sets=[[
      root,[left*.26,top+ch*.18],[left*.30,top*.73],[left*.44,top*.31],
      [w*.28,top*.24],[w*.50,top*.30],[w*.70,top*.23],
      [right+left*.45,top*.30],[right+left*.72,top+ch*.14],[right+left*.56,top+ch*.44]
    ],[
      root,[left*.59,top+ch*.20],[left*.67,top*.88],[left*.72,top*.65],
      [w*.28,top*.66],[w*.45,top*.62],[w*.63,top*.68],
      [right+left*.26,top*.63],[right+left*.40,top+ch*.08],[right+left*.27,top+ch*.25]
    ],[
      root,[left*.16,top+ch*.25],[left*.12,top+ch*.06],
      [left*.16,top*.70],[left*.30,top*.15],[w*.15,top*.10],[w*.18,top*.09]
    ]];
    if(w>=1100) {
      const near=root[0],span=near-20;
      sets=[[
        root,[near-12,top+ch*.20],[near-5,top*.75],[near+5,top*.28],
        [left+cw*.12,top*.18],[w*.50,top*.35],[right-cw*.06,top*.25],
        [right+left*.44,top*.68],[right+left*.82,top*.20],[w-left*.06,top+ch*.16],
        [right+left*.70,top+ch*.33],[right+left*.30,top+ch*.24]
      ],[
        root,[near+left*.14,top+ch*.20],[near+left*.17,top*.92],[near+left*.17,top*.65],
        [left+cw*.12,top*.64],[w*.46,top*.62],[right-cw*.08,top*.70],
        [right+left*.16,top*.68],[right+left*.20,top+ch*.45],[right+left*.52,top+ch*.37],
        [right+left*.77,top+ch*.33],[right+left*.91,top+ch*.57],[right+left*.94,top+ch*.86]
      ],[
        root,[span*.90,top+ch*.30],[span*.52,top+ch*.15],[span*.12,top+ch*.32],
        [span*.08,top+ch*.12],[span*.22,top*.75],[span*.58,top*.28],[span*.93,top*.72],[span*.92,top+ch*.13]
      ]];
    }
    if(mobile) {
      sets[0]=[root,[left*.28,top+ch*.10],[left*.30,top*.70],[left*.42,top*.24],[w*.30,top*.20],[w*.50,top*.29],[w*.72,top*.19],[w*.92,top*.27],[w*.94,top+ch*.09]];
      sets[1]=[root,[left*.58,top+ch*.12],[left*.62,top*.87],[left*.68,top*.62],[w*.28,top*.64],[w*.45,top*.61],[w*.65,top*.68],[w*.88,top*.63],[w*.92,top+ch*.035]];
      sets[2]=[root,[left*.16,top+ch*.18],[left*.12,top*.77],[left*.24,top*.14],[w*.15,top*.10],[w*.20,top*.09]];
    }
    const curve=(guides,knots=160)=>{
      const points=[];
      for(let i=0;i<guides.length-1;i++) {
        const a=guides[Math.max(0,i-1)],b=guides[i],c=guides[i+1],d=guides[Math.min(guides.length-1,i+2)];
        const steps=Math.floor((i+1)*(knots-1)/(guides.length-1))-Math.floor(i*(knots-1)/(guides.length-1));
        for(let j=0;j<steps;j++) {
          const t=j/steps,t2=t*t,t3=t2*t;
          const p=[0,1].map(k=>.5*(2*b[k]+(-a[k]+c[k])*t+(2*a[k]-5*b[k]+4*c[k]-d[k])*t2+(-a[k]+3*b[k]-3*c[k]+d[k])*t3));
          points.push(p);
        }
      }
      points.push([...guides.at(-1)]);
      return points;
    };
    const cross=(a,b,c)=>(b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0]);
    const intersects=(a,b,c,d)=>cross(a,b,c)*cross(a,b,d)<-.0001&&cross(c,d,a)*cross(c,d,b)<-.0001;
    const resample=(points,count=160)=>{
      const arc=[0];for(let i=1;i<points.length;i++)arc.push(arc[i-1]+distance(points[i-1],points[i]));
      let j=1;return Array.from({length:count},(_,i)=>{const p=arc.at(-1)*i/(count-1);while(j<arc.length-1&&arc[j]<p)j++;const t=(p-arc[j-1])/(arc[j]-arc[j-1]||1);return [0,1].map(k=>points[j-1][k]+(points[j][k]-points[j-1][k])*t);});
    };
    const axes=[],occupied=[];let future=[];
    const edge=(a,b)=>({a,b,minX:Math.min(a[0],b[0]),maxX:Math.max(a[0],b[0]),minY:Math.min(a[1],b[1]),maxY:Math.max(a[1],b[1]),root:distance(a,root)<10||distance(b,root)<10});
    const hits=(a,b)=>!(a.maxX<b.minX||a.minX>b.maxX||a.maxY<b.minY||a.minY>b.maxY)&&intersects(a.a,a.b,b.a,b.b);
    const pointDistance=(p,a,b)=>{const dx=b[0]-a[0],dy=b[1]-a[1],t=Math.max(0,Math.min(1,((p[0]-a[0])*dx+(p[1]-a[1])*dy)/(dx*dx+dy*dy||1)));return Math.hypot(p[0]-a[0]-dx*t,p[1]-a[1]-dy*t);};
    const close=(a,b,gap)=>!(a.maxX+gap<b.minX||a.minX-gap>b.maxX||a.maxY+gap<b.minY||a.minY-gap>b.maxY)&&
      (hits(a,b)||Math.min(pointDistance(a.a,b.a,b.b),pointDistance(a.b,b.a,b.b),pointDistance(b.a,a.a,a.b),pointDistance(b.b,a.a,a.b))<gap);
    const obstructs=(a,b)=>hits(a,b)||(!(b.junction&&(distance(a.a,b.junction)<14||distance(a.b,b.junction)<14))&&close(a,b,3));
    let visit=0;
    const edgeIndex=edges=>{
      const cells=new Map(),size=48;
      for(const e of edges)for(let x=Math.floor(e.minX/size);x<=Math.floor(e.maxX/size);x++)for(let y=Math.floor(e.minY/size);y<=Math.floor(e.maxY/size);y++){
        const key=x+','+y;if(!cells.has(key))cells.set(key,[]);cells.get(key).push(e);
      }
      return e=>{const result=[],id=++visit;for(let x=Math.floor((e.minX-3)/size);x<=Math.floor((e.maxX+3)/size);x++)for(let y=Math.floor((e.minY-3)/size);y<=Math.floor((e.maxY+3)/size);y++)for(const other of cells.get(x+','+y)||[])if(other.visit!==id){other.visit=id;result.push(other);}return result;};
    };
    let nearby=()=>[];
    const valid=points=>{
      const previous=[];
      for(let i=1;i<points.length;i++) {
        const a=points[i-1],b=points[i];if(!segmentFree(a,b))return false;
        if(i>1){const p=points[i-2],ux=a[0]-p[0],uy=a[1]-p[1],vx=b[0]-a[0],vy=b[1]-a[1],norm=Math.hypot(ux,uy)*Math.hypot(vx,vy);if(norm>.25&&(ux*vx+uy*vy)<norm*.64)return false;}
        const current=edge(a,b);
        for(let j=0;j<previous.length-2;j++)if(hits(current,previous[j]))return false;
        if(!current.root&&nearby(current).some(other=>!other.root&&obstructs(current,other)))return false;
        previous.push(current);
      }
      return true;
    };
    // Correlated steering inside a growth area, rather than a predefined loop.
    // Direction changes gradually; border pressure and existing stems steer it
    // toward unused space. A seed changes the route itself, not just its knots.
    const wander=(start,area,heading,count)=>{
      const guides=[[...start]],spanX=area.right-area.left,spanY=area.bottom-area.top;
      const stride=Math.max(18,Math.min(145,Math.max(spanX,spanY)*(count>7?.23:.29)));
      const obstacles=[...axes,...preservedBranches.filter(Boolean).map(b=>b.points)].flatMap(a=>a.filter((_,i)=>i%6===0)).filter(q=>distance(q,start)>12);
      const curl=(random()-.5)*.85,phase=random()*Math.PI*2;
      let turn=(random()-.5)*.7;
      for(let i=0;i<count;i++) {
        const from=guides.at(-1);turn=Math.max(-.52,Math.min(.52,turn*.68+curl*.28+Math.sin(phase+i*.76)*.16+(random()-.5)*.12));
        const desired=heading+turn;let best=null,score=-Infinity;
        for(const scale of [1,.72,.46])for(let j=0;j<13;j++) {
          const angle=desired+(j===0?0:(j%2?1:-1)*Math.ceil(j/2)*.15),step=stride*scale;
          const p=[from[0]+Math.cos(angle)*step,from[1]+Math.sin(angle)*step];
          if(p[0]<area.left||p[0]>area.right||p[1]<area.top||p[1]>area.bottom||!segmentFree(from,p,3))continue;
          const old=guides.slice(0,-2);
          if(old.some((q,k)=>k>0&&intersects(from,p,old[k-1],q)))continue;
          let clearance=75;for(const q of obstacles)clearance=Math.min(clearance,distance(q,p));
          const novelty=Math.min(stride,...old.map(q=>distance(q,p)));
          const edgeRoom=Math.min(p[0]-area.left,area.right-p[0],p[1]-area.top,area.bottom-p[1]);
          const value=Math.cos(angle-desired)*38+clearance*.40+novelty*.28+Math.min(25,edgeRoom)*.35+scale*7;
          if(value>score){score=value;best={p,angle};}
        }
        if(!best)break;
        guides.push(best.p);heading=best.angle;
      }
      return guides;
    };
    const route=(axis,base)=>{
      if(w>=1100) {
        if(axis===2) {
          const path=wander(root,{left:18,right:root[0]-9,top:18,bottom:root[1]+15},-Math.PI*(.48+random()*.50),9);
          if(path.length>=5)return path;
        } else {
          const upper=axis===0,low=top*(upper?.12:.58),high=top*(upper?.43:.80),phase=random()*Math.PI*2;
          const guides=base.slice(0,4).map(p=>[...p]);
          for(let i=0;i<3;i++) {
            const x=left+cw*(.10+i*.42)+(random()-.5)*Math.min(45,left*.08);
            const y=(low+high)/2+Math.sin(phase+i*(.7+random()*.5))*(high-low)*.40;
            guides.push([x,y]);
          }
          const start=[right+left*(upper?.14:.10),top*(upper?.28:.76)];
          guides.push(start);
          const path=wander(start,{left:right+18,right:w-18,top:upper?18:top*.60,bottom:upper?top+ch*.50:h-bottom-20},upper?-.12+random()*.65:.65+random()*.65,5);
          if(path.length>=4)return [...guides,...path.slice(1)];
        }
      }
      // Narrow screens leave little room beside the day. Vary the open upper
      // corridors more strongly while preserving the vertical climbing lanes.
      const phase=random()*Math.PI*2;
      return base.map((p,i)=>{
        if(i<3)return [...p];
        const x=p[0]+Math.sin(phase+i*.78)*Math.min(30,left*.24);
        const y=p[1]+Math.cos(phase+i*.69)*Math.min(25,top*.17);
        return [Math.max(9,Math.min(w-12,x)),Math.max(14,Math.min(h-bottom-20,y))];
      });
    };
    const bend=(points,axis,strength)=>{
      const wavelength=280+random()*160,phase=random()*Math.PI*2;
      let along=0;
      return points.map((p,i)=>{
        const a=points[Math.max(0,i-1)],b=points[Math.min(159,i+1)];
        if(i)along+=distance(points[i-1],p);
        const norm=distance(a,b)||1;
        const dx=Math.max(obstacle.left-p[0],0,p[0]-obstacle.right),dy=Math.max(obstacle.top-p[1],0,p[1]-obstacle.bottom);
        const room=Math.max(0,Math.min(p[0]-6,w-6-p[0],p[1]-7,h-bottom-10-p[1],Math.hypot(dx,dy)));
        const above=p[1]<obstacle.top,vertical=axis<2&&p[0]<obstacle.left;
        const amplitude=Math.min(above?30:vertical?38:54,room*.55)*(mobile?.7:1)*strength;
        const wave=Math.sin(phase+along/wavelength*Math.PI*2)*.78+Math.sin(phase*1.6+along/wavelength*Math.PI*1.14)*.22;
        const fade=Math.min(1,along/55),offset=amplitude*wave*fade*fade*(3-2*fade);
        return [p[0]-(b[1]-a[1])/norm*offset,p[1]+(b[0]-a[0])/norm*offset];
      });
    };
    for(const branch of preservedBranches)if(branch){const trace=branch.trace||branch.points;for(let i=1;i<trace.length;i++)occupied.push({...edge(trace[i-1],trace[i]),junction:branch.points[0]});}
    for(const [axis,base] of (pinnedAxes?[]:sets.entries())) {
      future=[];
      for(const path of previousAxes?.slice(axis+1)||[])for(let i=1;i<path.length;i++)future.push(edge(path[i-1],path[i]));
      nearby=edgeIndex([...occupied,...future]);
      let accepted=previousAxes&&lockedNodes[axis]===159?previousAxes[axis].map(p=>[...p]):null;
      for(let attempt=0;!accepted&&attempt<28;attempt++) {
        const guides=route(axis,base).map((p,i)=>{
          if(i===0)return [...p];
          if(i>3||axis===2)return [...p];
          const dx=(random()-.5)*Math.min(14,left*.07),dy=(random()-.5)*12;
          return [Math.max(8,Math.min(w-12,p[0]+dx)),Math.max(12,Math.min(h-bottom-18,p[1]+dy))];
        });
        const points=bend(curve(guides),axis,Math.max(.45,1-attempt*.025)),previous=previousAxes?.[axis],lock=lockedNodes[axis]??-1;
        // The living prefix is immutable. Only the hidden continuation can take
        // a new direction; a short blend keeps its tangent continuous.
        if(previous)for(let i=0;i<points.length;i++) {
          const t=Math.min(1,Math.max(0,(i-lock)/18)),weight=t*t*(3-2*t);
          points[i]=weight===0?[...previous[i]]:points[i].map((v,k)=>previous[i][k]+(v-previous[i][k])*weight);
        }
        if(valid(points)){accepted=points;break;}
      }
      if(!accepted){
        if(previousAxes){accepted=previousAxes[axis].map(p=>[...p]);}
        else {
        const fallback=resample(curve(base)),prefix=fallback.slice(0,2);
        for(let i=2;i<fallback.length;i++){prefix.push(fallback[i]);if(!valid(prefix)){prefix.pop();break;}}
        accepted=resample(prefix);
        }
      }
      axes.push(accepted);
      for(let i=1;i<accepted.length;i++)occupied.push(edge(accepted[i-1],accepted[i]));
    }
    if(pinnedAxes)axes.push(...pinnedAxes.map(a=>a.map(p=>[...p])));
    const arcs=axes.map(points=>{const a=[0];for(let i=1;i<points.length;i++)a.push(a[i-1]+distance(points[i-1],points[i]));return a;});
    const at=(axis,t)=>{
      const points=axes[axis],arc=arcs[axis],pos=t*arc.at(-1);
      let i=1;while(i<arc.length-1&&arc[i]<pos)i++;
      const f=(pos-arc[i-1])/(arc[i]-arc[i-1]||1),a=points[i-1],b=points[i];
      return {p:[a[0]+(b[0]-a[0])*f,a[1]+(b[1]-a[1])*f],angle:Math.atan2(b[1]-a[1],b[0]-a[0])};
    };
    const fractionNear=(axis,p)=>{
      let index=0,best=Infinity;axes[axis].forEach((q,i)=>{const d=distance(p,q);if(d<best){best=d;index=i;}});
      return arcs[axis][index]/arcs[axis].at(-1);
    };
    // Equal length budgets distribute the crown over twenty tasks. Keep the
    // resulting knot plan when a hidden tail changes, so old grafts stay valid.
    if(!stagePlan){
      const lengths=arcs.map(a=>a.at(-1)),grown=[Math.min(40,lengths[0]),0,0];
      const nodeAt=(axis,length)=>{const arc=arcs[axis];let i=1;while(i<159&&arc[i]<length)i++;return i-1+(length-arc[i-1])/(arc[i]-arc[i-1]||1);};
      stagePlan=[[0,0,0],grown.map((d,i)=>nodeAt(i,d))];
      const step=(lengths.reduce((s,d)=>s+d,0)-grown[0])/19;
      for(let n=2;n<=20;n++){
        let budget=step;
        for(let pass=0;pass<3&&budget>.001;pass++){
          const active=lengths.map((length,i)=>({i,room:length-grown[i],weight:length})).filter(a=>a.room>.001&&(a.i<2||n>=4));
          const weight=active.reduce((s,a)=>s+a.weight,0),portion=budget;
          for(const a of active){const added=Math.min(a.room,portion*a.weight/weight);grown[a.i]+=added;budget-=added;}
        }
        stagePlan.push(grown.map((d,i)=>nodeAt(i,Math.min(d,lengths[i]))));
      }
      stagePlan[20]=[159,159,159];
    }
    const stageNodes=n=>stagePlan[Math.min(20,Math.max(0,Math.floor(n)))].slice();
    const fractions=arcs.map(arc=>arc.map(d=>d/arc.at(-1)));
    const stages=n=>stageNodes(n).map((node,axis)=>{const i=Math.floor(node),f=node-i;return fractions[axis][i]+(i<159?(fractions[axis][i+1]-fractions[axis][i])*f:0);});
    const tips=preservedBranches.filter(Boolean).map(b=>b.points.at(-1)),branches=[];
    const bezier=(p,a,b,q)=>Array.from({length:13},(_,i)=>{
      const t=i/12,u=1-t;
      return [0,1].map(k=>u*u*u*p[k]+3*u*u*t*a[k]+3*u*t*t*b[k]+t*t*t*q[k]);
    });
    for(let i=0;i<count;i++) {
      if(preservedBranches[i]){const old=preservedBranches[i],branch={...old,axisT:fractions[old.axis][old.axisNode]};branches.push(branch);continue;}
      const n=i+1,nodes=stageNodes(n),axis=i===0?0:i===1?1:i===2?0:i===3?2:(i>11&&i%6===3?2:i%2);
      let index=i===0?1:Math.floor(nodes[axis]*(.82+(i%3)*.035));
      if(i>=12)index=Math.floor((axis===2?.18+.74*((i*.6180339)%1):.66+.30*((i*.6180339)%1))*159);
      index=Math.max(1,Math.min(index,Math.floor(nodes[axis]*.96)));
      const origin=(path,node)=>{const a=path[Math.max(0,node-1)],b=path[Math.min(path.length-1,node+1)];return {p:path[node],angle:Math.atan2(b[1]-a[1],b[0]-a[0])};};
      const primary={axis,axisNode:index,axisT:fractions[axis][index],origin:origin(axes[axis],index),attachment:0};
      const options=[primary],parentIndex=i-2,parent=branches[parentIndex];
      if(i>=6&&i%4===2&&parent&&!parent.dormant) {
        const parentNode=Math.floor((parent.points.length-1)*(.54+random()*.18)),arc=[0];
        for(let j=1;j<parent.points.length;j++)arc.push(arc[j-1]+distance(parent.points[j-1],parent.points[j]));
        if(arc.at(-1)>35)options.unshift({axis:parent.axis,axisNode:parent.axisNode,axisT:parent.axisT,parent:parentIndex,parentNode,attachment:arc[parentNode]/arc.at(-1),origin:origin(parent.points,parentNode)});
      }
      const blockers=[...axes,...branches.map(b=>b.trace||b.points),...preservedBranches.filter(Boolean).map(b=>b.trace||b.points)],edges=[];
      for(const path of blockers)for(let j=1;j<path.length;j++)edges.push(edge(path[j-1],path[j]));
      const neighbors=edgeIndex(edges);
      const clear=points=>{
        const self=[],trace=curve(points,49),origin=points[0];
        for(let j=1;j<trace.length;j++){
          if(!segmentFree(trace[j-1],trace[j],1.2))return false;
          const e=edge(trace[j-1],trace[j]),junction=distance(e.a,origin)<14||distance(e.b,origin)<14;
          for(const other of neighbors(e))if(hits(e,other)||(!junction&&close(e,other,2.5)))return false;
          for(let k=0;k<self.length-1;k++)if(hits(e,self[k]))return false;
          self.push(e);
        }
        return true;
      };
      let chosen=null;
      for(const option of options) {
        const o=option.origin,reach=i===0?15:i<3?24+random()*12:mobile?24+random()*14:(option.parent!==undefined?40+random()*30:55+random()*60),direction=random()<.5?-1:1;
        search:for(const scale of [1,.78,.55,.32])for(const sign of [direction,-direction])for(const spread of [.45,.72,1.02]) {
          const length=reach*scale,angle=o.angle+sign*spread,end=[o.p[0]+Math.cos(angle)*length,o.p[1]+Math.sin(angle)*length];
          if(!free(end,mobile?5:8)||tips.some(p=>distance(p,end)<(mobile?9:14)))continue;
          const departure=o.angle+sign*.18,tip=angle+sign*(i%3===0?-.20:.48);
          const a=[o.p[0]+Math.cos(departure)*length*.42,o.p[1]+Math.sin(departure)*length*.42];
          const b=[end[0]-Math.cos(tip)*length*.30,end[1]-Math.sin(tip)*length*.30];
          const points=bezier(o.p,a,b,end);
          if(clear(points)){chosen={...option,points,trace:curve(points,49),target:end};break search;}
        }
        if(chosen)break;
      }
      // If a new shoot cannot fit, its task still needs one visible organ.
      // Attach it directly to a free point of already-supported main wood.
      if(!chosen){
        let best=null;
        for(let a=0;a<axes.length;a++)for(let node=1;node<=Math.floor(nodes[a]);node++){
          const o=origin(axes[a],node),p=o.p;
          if(arcs[a][node]<12||!free(p,mobile?7:9))continue;
          const gap=Math.min(100,...tips.map(t=>distance(t,p)));
          const score=gap-distance(p,primary.origin.p)*.025;
          if(!best||score>best.score)best={axis:a,axisNode:node,axisT:fractions[a][node],origin:o,attachment:0,score};
        }
        const graft=best||primary,p=graft.origin.p;
        chosen={...graft,points:Array.from({length:13},()=>[...p]),target:p,dormant:true,markerOnly:true,markerAngle:graft.origin.angle*180/Math.PI};
        delete chosen.score;
      }
      delete chosen.origin;
      branches.push({...chosen,activation:n});tips.push(chosen.target);
    }
    return {trunk:axes[0],axes,branches,obstacle,free,segmentFree,mobile,stages,stageNodes,stagePlan,axisFractions:fractions,axisLengths:arcs.map(a=>a.at(-1)),nodes:axes.reduce((n,a)=>n+a.length,0),attractors:count};
  }
  function transitionSafe(from,to) {
    const cross=(a,b,c)=>(b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0]);
    for(const t of [.1,.2,.3,.4,.5,.6,.7,.8,.9,1]) {
      const axes=to.axes.map((a,i)=>a.map((p,j)=>p.map((v,k)=>v*t+from.axes[i][j][k]*(1-t)))),root=axes[0][0],edges=[];
      for(const path of axes) {
        const self=[];
        for(let i=1;i<path.length;i++) {
          const a=path[i-1],b=path[i];if(!to.segmentFree(a,b,1))return false;
          const e={a,b,x0:Math.min(a[0],b[0]),x1:Math.max(a[0],b[0]),y0:Math.min(a[1],b[1]),y1:Math.max(a[1],b[1]),root:distance(a,root)<10||distance(b,root)<10};
          const crosses=other=>{
            if(e.root||other.root||e.x1<other.x0||e.x0>other.x1||e.y1<other.y0||e.y0>other.y1)return false;
            return cross(a,b,other.a)*cross(a,b,other.b)<-.0001&&cross(other.a,other.b,a)*cross(other.a,other.b,b)<-.0001;
          };
          for(const other of edges)if(crosses(other))return false;
          for(let j=0;j<self.length-2;j++)if(crosses(self[j]))return false;
          self.push(e);
        }
        edges.push(...self);
      }
    }
    return true;
  }

export { build, transitionSafe };
