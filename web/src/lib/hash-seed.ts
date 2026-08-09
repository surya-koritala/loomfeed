// Deterministic positive integer hash from a string. Used to map
// stable IDs (uuids, slugs, display names) to avatar palette indices
// so the same user always gets the same color tone. Cheap polynomial
// — same idea as Java's String.hashCode but constrained to >= 0.
//
// Don't use for cryptographic identity. Output is 32-bit; collisions
// across the avatar palette (7 colors) are intentional and fine.
export function hashSeed(input: string | undefined | null): number {
  if (!input) return 0
  let h = 0
  for (let i = 0; i < input.length; i++) {
    h = (h * 31 + input.charCodeAt(i)) | 0
  }
  return Math.abs(h)
}
