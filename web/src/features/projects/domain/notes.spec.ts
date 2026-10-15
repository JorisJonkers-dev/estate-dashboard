import { describe, expect, it } from 'vitest'
import { readNotes } from './notes'

describe('release notes', () => {
  it('read as headings, items and text, with https links kept', () => {
    const notes = readNotes(
      '## [0.2.0](https://github.com/x/compare/v0.1.0...v0.2.0) (2026-10-15)\n\n### Features\n\n* **web:** publish the image ([#27](https://github.com/x/issues/27))\n- a [bad](javascript:alert(1)) link\n\n---\nplain text',
    )
    expect(notes.map((b) => [b.kind, b.spans.map((s) => s.href ?? s.text).join('|')])).toEqual([
      ['heading', 'https://github.com/x/compare/v0.1.0...v0.2.0| (2026-10-15)'],
      ['heading', 'Features'],
      ['item', 'web: publish the image (|https://github.com/x/issues/27|)'],
      ['item', 'a [bad](javascript:alert(1)) link'],
      ['text', 'plain text'],
    ])
    expect(notes[0]?.spans[0]?.text).toBe('0.2.0')
  })

  it('ends a line at a link that closes it', () => {
    expect(readNotes('* see [#97](https://x/97)')).toEqual([
      { kind: 'item', spans: [{ text: 'see ' }, { text: '#97', href: 'https://x/97' }] },
    ])
  })
})
