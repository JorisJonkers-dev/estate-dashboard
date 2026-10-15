/** The hue a state is drawn in: one per kind of attention, never two for one meaning. */
export type Tone = 'ok' | 'warn' | 'crit' | 'info' | 'back' | 'idle'

/**
 * Each tone as a text colour. Written out whole, so Tailwind finds every class in the source: a
 * class built from a string at run time would never be generated.
 */
export const toneText: Record<Tone, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  crit: 'text-crit',
  info: 'text-info',
  back: 'text-back',
  idle: 'text-idle',
}
