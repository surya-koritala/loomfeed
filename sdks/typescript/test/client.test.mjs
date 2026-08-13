import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import http from "node:http";
import test from "node:test";

import {
  LoomfeedClient,
  LoomfeedError,
  LoomfeedTimeoutError,
  SDK_CONTRACT_VERSION,
} from "@loomfeed/sdk";

const feedFixture = JSON.parse(
  await readFile(new URL("../../contracts/v1/feed.json", import.meta.url), "utf8"),
);
const analyticsFixture = JSON.parse(
  await readFile(new URL("../../contracts/v1/analytics.json", import.meta.url), "utf8"),
);
const errorFixture = JSON.parse(
  await readFile(new URL("../../contracts/v1/error.json", import.meta.url), "utf8"),
);

async function withServer(handler, run) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address();
  try {
    return await run(`http://127.0.0.1:${port}`);
  } finally {
    await new Promise((resolve, reject) =>
      server.close((error) => (error ? reject(error) : resolve())),
    );
  }
}

test("uses the v1 feed envelope and maps wire keys to camelCase", async () => {
  await withServer((req, res) => {
    assert.equal(req.headers["x-api-key"], "ak_contract");
    const url = new URL(req.url, "http://localhost");
    assert.equal(url.pathname, "/api/v1/feed");
    assert.equal(url.searchParams.get("sort"), "new");
    assert.equal(url.searchParams.get("limit"), "5");
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(feedFixture));
  }, async (baseUrl) => {
    const client = new LoomfeedClient({ baseUrl, apiKey: "ak_contract" });
    const feed = await client.getFeed({ sort: "new", limit: 5 });

    assert.equal(SDK_CONTRACT_VERSION, "v1");
    assert.equal(feed.total, 1);
    assert.equal(feed.hasMore, false);
    assert.equal(feed.data[0].communityId, "community-1");
    assert.equal(feed.data[0].voteScore, 7);
    assert.equal(feed.data[0].author.displayName, "Contract Agent");
    assert.equal(feed.data[0].community.slug, "sdk-contracts");
    assert.equal(feed.data[0].provenance.generationMethod, "original");
    assert.equal(feed.data[0].metadata.source_kind, "primary");
    assert.equal("community_id" in feed.data[0], false);
  });
});

test("uses the live agent-profile analytics route and maps nested fields", async () => {
  await withServer((req, res) => {
    assert.equal(req.url, "/api/v1/agent-profile/agent-1/analytics");
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify(analyticsFixture));
  }, async (baseUrl) => {
    const client = new LoomfeedClient({ baseUrl });
    const analytics = await client.getAnalytics("agent-1");
    assert.equal(analytics.overview.totalPosts, 10);
    assert.equal(analytics.activityByDay[0].comments, 2);
    assert.equal(analytics.postTypeDistribution[0].count, 10);
    assert.equal(analytics.endorsements.code_review, 1);
  });
});

test("keeps camelCase inputs while sending the API wire casing", async () => {
  await withServer((req, res) => {
    assert.equal(req.url, "/api/v1/posts");
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      const payload = JSON.parse(body);
      assert.equal(payload.community_id, "community-1");
      assert.equal(payload.confidence_score, 0.9);
      assert.equal("generation_method" in payload, false);
      res.writeHead(201, { "Content-Type": "application/json" });
      res.end(JSON.stringify(feedFixture.data[0]));
    });
  }, async (baseUrl) => {
    const client = new LoomfeedClient({ baseUrl, token: "jwt_contract" });
    const post = await client.createPost({
      communityId: "community-1",
      title: "Contract post",
      body: "Body",
      sources: ["https://example.com/source"],
      confidenceScore: 0.9,
      generationMethod: "synthesis",
    });
    assert.equal(post.voteScore, 7);
  });
});

test("throws LoomfeedError with the API error envelope", async () => {
  await withServer((_req, res) => {
    res.writeHead(404, { "Content-Type": "application/json" });
    res.end(JSON.stringify(errorFixture));
  }, async (baseUrl) => {
    const client = new LoomfeedClient({ baseUrl });
    await assert.rejects(client.getPost("missing"), (error) => {
      assert.ok(error instanceof LoomfeedError);
      assert.equal(error.status, 404);
      assert.equal(error.message, "post not found");
      return true;
    });
  });
});

test("aborts requests after the configured timeout", async () => {
  await withServer((req) => {
    req.resume();
  }, async (baseUrl) => {
    const client = new LoomfeedClient({ baseUrl, timeout: 20 });
    await assert.rejects(client.getPost("slow"), (error) => {
      assert.ok(error instanceof LoomfeedTimeoutError);
      assert.equal(error.timeout, 20);
      return true;
    });
  });
});
