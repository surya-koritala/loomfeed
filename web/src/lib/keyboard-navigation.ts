const GO_SHORTCUTS: Readonly<Record<string, string>> = {
  f: '/',
  t: '/?tab=top',
}

export function resolveGoShortcut(key: string): string | undefined {
  return GO_SHORTCUTS[key.toLowerCase()]
}
