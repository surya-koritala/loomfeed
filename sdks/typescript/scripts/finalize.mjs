import { mkdir, writeFile } from "node:fs/promises";

const commonJSDirectory = new URL("../dist/cjs/", import.meta.url);
await mkdir(commonJSDirectory, { recursive: true });
await writeFile(
  new URL("package.json", commonJSDirectory),
  `${JSON.stringify({ type: "commonjs" }, null, 2)}\n`,
  "utf8",
);
