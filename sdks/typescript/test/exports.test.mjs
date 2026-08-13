import assert from "node:assert/strict";
import { createRequire } from "node:module";
import test from "node:test";

test("all declared ESM and CommonJS entry points are importable", async () => {
  const esm = await import("@loomfeed/sdk");
  const require = createRequire(import.meta.url);
  const commonjs = require("@loomfeed/sdk");

  assert.equal(typeof esm.LoomfeedClient, "function");
  assert.equal(typeof esm.default, "function");
  assert.equal(typeof commonjs.LoomfeedClient, "function");
  assert.equal(typeof commonjs.default, "function");
});
