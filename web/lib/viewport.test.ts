import test from "node:test";
import assert from "node:assert/strict";
import {cardBounds, selectCamera, CameraInitialization, OverviewCamera, type CardGeometry} from "./viewport.ts";
const card=(id:string,x=0,y=0,width=300,height=80,path=id):CardGeometry=>({id,path,x,y,width,height});
test("empty, unmeasured and nonfinite geometry cannot move the camera",()=>{
  for(const cards of [[],[card("a",0,0,0)],[card("a",NaN)]]) assert.equal(selectCamera(cards,{width:800,height:600},.08),null);
  assert.equal(selectCamera([card("a")],{width:0,height:600},.08),null);
  assert.equal(selectCamera([card("a")],{width:800,height:Infinity},.08),null);
});
test("fits use measured card geometry and inclusive readable boundaries",()=>{
  assert.deepEqual(cardBounds([card("a",-10,-20,200,100),card("b",500,300,400,200)]),{x:-10,y:-20,width:910,height:520});
  assert.deepEqual(selectCamera([card("a")],{width:800,height:600},0),{x:227.5,y:254,zoom:1.15});
  const cards=[card("a",0,0,1000,100)];
  assert.equal(selectCamera(cards,{width:850,height:600},0)?.zoom,.85);
  assert.equal(selectCamera(cards,{width:1150,height:600},0)?.zoom,1.15);
  assert.equal(selectCamera(cards,{width:1100,height:600},.1)?.zoom,1);
});
test("cropping anchors by squared center distance, path then ID with focal override",()=>{
  const cards=[card("z",0,0,100,100,"a"),card("a",10000,0,100,100,"a")];
  const camera=selectCamera(cards,{width:800,height:600},.08)!;
  assert.equal(camera.x,400-10050*.85);
  assert.deepEqual(selectCamera([...cards].reverse(),{width:800,height:600},.08),camera);
  const paths=[card("z",0,0,100,100,"a".repeat(2000)),card("a",10000,0,100,100,"z")];
  assert.equal(selectCamera(paths,{width:800,height:600},0)?.x,400-50*.85);
  assert.equal(selectCamera(paths,{width:800,height:600},0,"readable","a")?.x,400-10050*.85);
  assert.deepEqual(selectCamera(paths,{width:800,height:600},0,"readable","missing"),selectCamera(paths,{width:800,height:600},0));
});
test("Fit all includes sparse and huge graphs below readable zoom",()=>{
  const cards=[card("left",0,0),card("right",10000,2000)];
  const all=selectCamera(cards,{width:800,height:600},.08,"all")!;
  const bounds=cardBounds(cards)!;
  assert(all.zoom<.85);assert(all.x+bounds.x*all.zoom>=0);assert(all.x+(bounds.x+bounds.width)*all.zoom<=800);
  assert.equal(selectCamera([card("far",1e9,1e9)],{width:800,height:600},.08)?.zoom,1.15);
});
test("camera initialization is one-shot per view, independent of resize and unrelated updates",()=>{
  const state=new CameraInitialization();assert(state.pending(1));state.complete(1);
  for(const update of ["selection","measure","resize","preferences","detail"]) assert.equal(state.pending(1),false,update);
  assert(state.pending(2));state.complete(2);assert.equal(state.pending(2),false);
});

test("a graph that fits with React Flow padding is not cropped to one card", () => {
  const cards = [card("a", 0, 0, 100, 100), card("b", 900, 0, 100, 100)];
  const view = selectCamera(cards, { width: 1000, height: 600 }, .08)!;
  assert.deepEqual(view, { x: 37, y: 253.7, zoom: .926 });
  assert.equal(view.x + 500 * view.zoom, 500);
  const cropped = selectCamera(cards, { width: 800, height: 600 }, .08, "readable", "b")!;
  assert.equal(cropped.zoom, .85);
  assert.equal(cropped.x + 950 * cropped.zoom, 400);
});

test("path and ID ties use lexical order rather than locale collation", () => {
  const cards = [card("left", 0, 0, 100, 100, "ä"), card("right", 10000, 0, 100, 100, "z")];
  const expected = { x: 400 - 10050 * .85, y: 300 - 50 * .85, zoom: .85 };
  assert.deepEqual(selectCamera(cards, { width: 800, height: 600 }, .08), expected);
  assert.deepEqual(selectCamera([...cards].reverse(), { width: 800, height: 600 }, .08), expected);
  assert.deepEqual(selectCamera(cards.map(c => ({ ...c, path: "same", id: c.path })), { width: 800, height: 600 }, .08), expected);
});

test("unusable geometry leaves initialization pending until a measured view is available", () => {
  const initialization = new CameraInitialization();
  const initialize = (width: number, cardWidth: number) => {
    if (!initialization.pending("focus")) return null;
    const camera = selectCamera([card("f", 0, 0, cardWidth)], { width, height: 600 }, .12);
    if (camera) initialization.complete("focus");
    return camera;
  };
  assert.equal(initialize(0, 300), null);
  assert.equal(initialize(800, 0), null);
  assert(initialization.pending("focus"));
  assert(initialize(800, 300));
  assert.equal(initialize(1200, 300), null);
});

test("return consumes the exact overview camera so evidence reveal cannot replay it after panning", () => {
  const saved = new OverviewCamera();
  const original = { x: -123.125, y: 456.875, zoom: .42 };
  saved.capture(original);
  const restored = saved.peek()!;
  assert.deepEqual(restored, original);
  assert.notEqual(restored, original);
  saved.consume(restored);
  const afterPan = { x: 999, y: -222, zoom: 1.73 };
  // Evidence reveal calls Return even when already in overview.
  assert.deepEqual(saved.peek() ?? afterPan, afterPan);
  saved.capture(afterPan);
  saved.consume(restored);
  assert.deepEqual(saved.peek(), afterPan);
});
