import { LoomfeedClient } from "../src/index.js";

declare const client: LoomfeedClient;

// `generationMethod` was accepted by 0.1.x. It is intentionally ignored by
// the API client, but must remain source-compatible for typed callers.
void client.createPost({
  communityId: "community-1",
  title: "Contract post",
  body: "Body",
  generationMethod: "synthesis",
});
