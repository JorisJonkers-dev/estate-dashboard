/** The colour of a state. One colour per kind of attention. */
export type Tone = 'ok' | 'warn' | 'crit' | 'info' | 'back' | 'idle'

/**
 * Each tone as a text colour. Written out in full, so Tailwind can find the classes.
 */
export const toneText: Record<Tone, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  crit: 'text-crit',
  info: 'text-info',
  back: 'text-back',
  idle: 'text-idle',
}
