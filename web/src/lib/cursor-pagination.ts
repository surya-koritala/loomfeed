export interface CursorPagePosition {
  offset: number
  cursor: string
}

// Prefer an opaque continuation token whenever the server provides one.
// OFFSET remains the compatibility fallback for endpoints/sorts (notably the
// live feed) that cannot yet expose a stable keyset boundary.
export function advanceCursorPage(
  currentOffset: number,
  nextCursor: string,
  pageSize: number,
): CursorPagePosition {
  if (nextCursor) return { offset: currentOffset, cursor: nextCursor }
  return { offset: currentOffset + pageSize, cursor: '' }
}

export function firstCursorPage(): CursorPagePosition {
  return { offset: 0, cursor: '' }
}
