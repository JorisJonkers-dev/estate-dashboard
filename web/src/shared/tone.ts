/** Colour of each state. */
export type Tone = 'ok' | 'warn' | 'crit' | 'info' | 'back' | 'idle'

/** Text class for each tone, written out in full so Tailwind finds them. */
export const toneText: Record<Tone, string> = {
  ok: 'text-ok',
  warn: 'text-warn',
  crit: 'text-crit',
  info: 'text-info',
  back: 'text-back',
  idle: 'text-idle',
}
