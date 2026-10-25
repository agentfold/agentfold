/** Non-empty trimmed lines of text. */
export function parseLines(text: string): string[] {
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
}

/** KEY=VALUE lines to an object; lines without "=" are skipped. */
export function parsePairs(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of parseLines(text)) {
    const at = line.indexOf('=')
    if (at > 0) {
      out[line.slice(0, at).trim()] = line.slice(at + 1).trim()
    }
  }
  return out
}

/** An object as KEY=VALUE lines. */
export function formatPairs(pairs: Record<string, string>): string {
  return Object.entries(pairs)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}
