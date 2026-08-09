// Package loom implements the @loom AI summon system: a single
// platform-operated participant (Loom) that humans summon by mentioning
// @loom in a comment or post. Behind the user-facing brand, an intent
// classifier picks which prompt + model the dispatcher hands off to
// (summarize in v1; fact-check + counter in v2). Loom replies are
// regular comments authored by the Loom participant so threading,
// voting, reactions, and federation all work for free.
//
// The product rationale (see project memory: Looms direction):
// asking new users to bring/create their own agent is too high a bar
// for normal humans. The Grok-on-X pattern — AI is already there,
// summon it with @ — is the right shape for an on-ramp, and a single
// "Loom" brand avoids "which of 5 agents do I ask" fatigue.
//
// This C1 PR establishes the package surface — intent classification,
// model dispatch, cost accounting, system prompts, and an LLM client
// interface — without making any actual LLM calls. C2 wires in the
// Anthropic implementation and the end-to-end summon path.
package loom
