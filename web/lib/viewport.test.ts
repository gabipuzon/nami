import assert from "node:assert/strict";
import test from "node:test";
import {fitAllViewport,readableViewport,type CardRect} from "./viewport.ts";
const card=(id:string,x:number,y:number,path=id):CardRect=>({id,path,x,y,width:304,height:82});
test("empty/invalid stages do not initialize a camera",()=>{
 assert.equal(readableViewport([],1000,700),null);
 assert.equal(readableViewport([card("a",0,0)],0,700),null);
 assert.equal(fitAllViewport([],1000,700),null);
});
test("small graph has readable automatic zoom",()=>{
 assert.equal(readableViewport([card("a",0,0)],1200,700)?.zoom,1.15);
});
test("large graph crops to nearest card and stable path/ID tie",()=>{
 const cards=[card("farLeft",0,0),card("z",2000,0,"b"),card("a",2000,0,"a"),card("farRight",4000,0)];
 const one=readableViewport(cards,700,400)!;
 assert.equal(one.zoom,.85);
 assert.equal(one.x,350-(2000+152)*.85);
 assert.deepEqual(one,readableViewport([...cards].reverse(),700,400));
});
test("focus centers its requested focal card and fit all may be smaller than readable min",()=>{
 const cards=[card("a",0,0),card("target",8000,4000)];
 const view=readableViewport(cards,900,500,"target")!;
 assert.equal(view.zoom,.85);
 assert.equal(view.x,450-(8000+152)*.85);
 assert((fitAllViewport(cards,900,500)?.zoom??1)<.85);
});
test("sparse long paths remain stable across input ordering",()=>{
 const cards=[card("left",-1000,0,"/a/".repeat(500)),card("right",3000,0,"/b/".repeat(500))];
 assert.deepEqual(readableViewport(cards,900,500),readableViewport([...cards].reverse(),900,500));
});
