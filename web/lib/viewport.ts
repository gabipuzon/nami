export interface CardRect {
  id: string;
  path: string;
  x: number;
  y: number;
  width: number;
  height: number;
}
export interface Camera { x: number; y: number; zoom: number }

const MIN_READABLE = 0.85;
const MAX_READABLE = 1.15;
const center = (c: CardRect) => ({x:c.x+Math.max(1,c.width)/2,y:c.y+Math.max(1,c.height)/2});
const viewport = (x:number,y:number,z:number,w:number,h:number):Camera => ({x:w/2-x*z,y:h/2-y*z,zoom:z});

function metrics(cards:readonly CardRect[],w:number,h:number,padding:number) {
  if (!cards.length || !Number.isFinite(w) || !Number.isFinite(h) || w<=0 || h<=0 ||
    cards.some(c=>![c.x,c.y,c.width,c.height].every(Number.isFinite))) return null;
  const minX=Math.min(...cards.map(c=>c.x));
  const maxX=Math.max(...cards.map(c=>c.x+Math.max(1,c.width)));
  const minY=Math.min(...cards.map(c=>c.y));
  const maxY=Math.max(...cards.map(c=>c.y+Math.max(1,c.height)));
  return {x:(minX+maxX)/2,y:(minY+maxY)/2,
    fit:Math.min(w*(1-2*padding)/Math.max(1,maxX-minX),h*(1-2*padding)/Math.max(1,maxY-minY))};
}

export function fitAllViewport(cards:readonly CardRect[],width:number,height:number,padding=0.08):Camera|null {
  const m=metrics(cards,width,height,padding);
  return m?viewport(m.x,m.y,Math.max(0.01,Math.min(2,m.fit)),width,height):null;
}
export function readableViewport(cards:readonly CardRect[],width:number,height:number,focalID?:string,padding=0.08):Camera|null {
  const m=metrics(cards,width,height,padding);
  if (!m) return null;
  if (m.fit>=MIN_READABLE) return viewport(m.x,m.y,Math.min(MAX_READABLE,m.fit),width,height);
  const focal=cards.find(c=>c.id===focalID) ?? [...cards].sort((a,b)=>{
    const ac=center(a),bc=center(b);
    const ad=(ac.x-m.x)**2+(ac.y-m.y)**2,bd=(bc.x-m.x)**2+(bc.y-m.y)**2;
    return ad-bd || a.path.localeCompare(b.path) || a.id.localeCompare(b.id);
  })[0];
  const c=center(focal);
  return viewport(c.x,c.y,MIN_READABLE,width,height);
}
