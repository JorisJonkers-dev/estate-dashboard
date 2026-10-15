/** A piece of a line of notes: text, or a link. */
export interface Span {
  text: string
  href?: string
}

/** One block of release notes. */
export interface Block {
  kind: 'heading' | 'item' | 'text'
  spans: Span[]
}

/** A link in Markdown: [text](https://...). Only https links are kept as links. */
const link = /\[([^\]]+)\]\((https:\/\/[^)\s]+)\)/g

function spans(line: string): Span[] {
  const out: Span[] = []
  let at = 0
  for (const m of line.matchAll(link)) {
    if (m.index > at) out.push({ text: line.slice(at, m.index) })
    out.push({ text: String(m[1]), href: String(m[2]) })
    at = m.index + m[0].length
  }
  if (at < line.length) out.push({ text: line.slice(at) })
  return out.map((s) => ({ ...s, text: s.text.replaceAll('**', '') }))
}

/** Reads release notes into headings, items and text. Nothing becomes markup. */
export function readNotes(markdown: string): Block[] {
  const blocks: Block[] = []
  for (const raw of markdown.split('\n')) {
    const line = raw.trim()
    if (line === '' || line === '---') continue
    const heading = /^#{1,6}\s+(.*)$/.exec(line)
    const item = /^[*-]\s+(.*)$/.exec(line)
    if (heading) blocks.push({ kind: 'heading', spans: spans(String(heading[1])) })
    else if (item) blocks.push({ kind: 'item', spans: spans(String(item[1])) })
    else blocks.push({ kind: 'text', spans: spans(line) })
  }
  return blocks
}
