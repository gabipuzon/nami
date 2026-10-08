import { cp, mkdir, rm } from "node:fs/promises";
const target = new URL("../../internal/webui/assets/", import.meta.url);
await rm(target, { recursive: true, force: true });
await mkdir(target, { recursive: true });
await cp(new URL("../out/", import.meta.url), target, { recursive: true });
