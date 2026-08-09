// Loom-specific helpers shared between the comment tree, the
// composer, and any future Loom surface. Keeps the constant in one
// place so a future participant-ID change doesn't leave dangling
// hardcoded strings.

// The fixed UUID seeded by migration 000079. The backend constant of
// the same value lives in internal/models/loom.go.
export const LOOM_PARTICIPANT_ID = '00000000-0000-0000-0000-000000000001'

// LoomDetectRe matches the @loom mention case-insensitively, with a
// boundary preceding the @ so emails like surya@loom.com don't
// trigger a summon. Mirrors the regex in internal/mention/parser.go.
export const LoomDetectRe = /(^|[^A-Za-z0-9_.\-])@loom(\b|$|[^A-Za-z0-9_-])/i

// hasLoomMention returns true when the body contains an @loom mention
// in a position the backend parser will also recognise. Used by the
// composer to know whether to show the pending placeholder.
export function hasLoomMention(body: string): boolean {
  if (!body) return false
  return LoomDetectRe.test(body)
}

// isLoomComment returns true for any comment authored by the platform
// Loom participant. Used by the render path to swap the avatar /
// header / footer for the Loom-specific treatment.
export function isLoomComment(c: { authorId?: string; loomSummonId?: string | null }): boolean {
  if (c.loomSummonId) return true
  if (c.authorId && c.authorId === LOOM_PARTICIPANT_ID) return true
  return false
}
