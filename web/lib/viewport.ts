export interface Camera { x: number; y: number; zoom: number }
export interface CardGeometry { id: string; path: string; x: number; y: number; width: number; height: number }
export interface CanvasSize { width: number; height: number }
export interface CardBounds { x: number; y: number; width: number; height: number }

export function cardBounds(cards: readonly CardGeometry[]): CardBounds | null {
  if (!cards.length || cards.some(c => ![c.x,c.y,c.width,c.height].every(Number.isFinite) || c.width <= 0 || c.height <= 0)) return null;
  const x = Math.min(...cards.map(c => c.x));
  const y = Math.min(...cards.map(c => c.y));
  return { x, y, width: Math.max(...cards.map(c => c.x + c.width)) - x, height: Math.max(...cards.map(c => c.y + c.height)) - y };
}
function center(x: number, y: number, zoom: number, size: CanvasSize): Camera {
  return { x: size.width / 2 - x * zoom, y: size.height / 2 - y * zoom, zoom };
}
function fitZoom(bounds: CardBounds, size: CanvasSize, padding: number): number {
  // Match the installed React Flow numeric padding conversion, including rounding.
  const available = (length: number) => length - 2 * Math.floor((length - length / (1 + padding)) * .5);
  return Math.min(available(size.width) / bounds.width, available(size.height) / bounds.height);
}
export function selectCamera(cards: readonly CardGeometry[], size: CanvasSize, padding: number, mode: "readable" | "all" = "readable", focalID?: string): Camera | null {
  const bounds = cardBounds(cards);
  if (!bounds || ![size.width, size.height, padding].every(Number.isFinite) || size.width <= 0 || size.height <= 0 || padding < 0) return null;
  const fit = fitZoom(bounds, size, padding);
  const cx = bounds.x + bounds.width / 2;
  const cy = bounds.y + bounds.height / 2;
  if (mode === "all" || fit >= .85) {
    const zoom = Math.min(1.15, Math.max(mode === "all" ? .01 : .85, fit));
    return center(cx, cy, zoom, size);
  }
  const distance = (c: CardGeometry) => (c.x + c.width / 2 - cx) ** 2 + (c.y + c.height / 2 - cy) ** 2;
  const lexical = (a: string, b: string) => a < b ? -1 : a > b ? 1 : 0;
  const anchor = cards.find(c => c.id === focalID) ?? [...cards].sort((a, b) =>
    distance(a) - distance(b) || lexical(a.path, b.path) || lexical(a.id, b.id)
  )[0];
  return center(anchor.x + anchor.width / 2, anchor.y + anchor.height / 2, .85, size);
}

// A completed view never becomes pending again because its geometry changes.
export class CameraInitialization {
  private completed: string | number | undefined;
  pending(view: string | number): boolean { return this.completed !== view; }
  complete(view: string | number): void { this.completed = view; }
}

// A saved overview camera belongs to one focus excursion. Evidence reveal also
// returns to overview, so consuming the camera prevents replaying an old pan.
export class OverviewCamera {
  private saved: Camera | null = null;

  capture(camera: Camera): void { this.saved = { ...camera }; }
  peek(): Camera | null { return this.saved; }
  consume(camera: Camera): void {
    if (this.saved === camera) this.saved = null;
  }
}
