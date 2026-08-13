import { rm } from "node:fs/promises";

const dist = new URL("../dist/", import.meta.url);
if (!dist.pathname.endsWith("/sdks/typescript/dist/")) {
  throw new Error(`refusing to clean unexpected path: ${dist.pathname}`);
}

await rm(dist, { recursive: true, force: true });
