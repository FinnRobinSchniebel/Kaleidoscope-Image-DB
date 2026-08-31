export interface ParsedSearchQuery {
  words: string[]
  tags: string[]
  titles: string[]
  authors: string[]
  sources: string[]
}

const PREFIXES: { prefix: string; key: keyof ParsedSearchQuery }[] = [
  { prefix: "tag:", key: "tags" },
  { prefix: "title:", key: "titles" },
  { prefix: "author:", key: "authors" },
  { prefix: "source:", key: "sources" },
]

// Splits raw on whitespace. A token whose lowercased form starts with a
// recognized prefix has the prefix stripped and the remainder (if non-empty)
// added to that category; every other token is a bare word.
export function parseSearchQuery(raw: string): ParsedSearchQuery {
  const result: ParsedSearchQuery = { words: [], tags: [], titles: [], authors: [], sources: [] }
  const tokens = raw.trim().split(/\s+/).filter(Boolean)

  for (const token of tokens) {
    const lower = token.toLowerCase()
    const match = PREFIXES.find(p => lower.startsWith(p.prefix))
    if (match) {
      const remainder = token.slice(match.prefix.length)
      if (remainder !== "") result[match.key].push(remainder)
      continue
    }
    result.words.push(token)
  }
  return result
}
