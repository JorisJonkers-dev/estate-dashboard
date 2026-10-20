/** One block of a status strip: its colour, and what the tooltip says of it. */
export interface StripBlock {
  tone: 'ok' | 'info' | 'warn' | 'crit' | 'empty'
  label: string
}
