/** What the history needs of an event to describe it. */
export interface EventTimes {
  status: 'firing' | 'resolved'
  startsAt: string
  endsAt?: string
}

/** How long an alert fired, in the two largest units that say something: "45 s", "2 h 5 min". */
export function formatDuration(milliseconds: number): string {
  const seconds = Math.max(0, Math.round(milliseconds / 1000))
  const units: [number, string][] = [
    [Math.floor(seconds / 86_400), 'd'],
    [Math.floor((seconds % 86_400) / 3600), 'h'],
    [Math.floor((seconds % 3600) / 60), 'min'],
    [seconds % 60, 's'],
  ]
  const first = units.findIndex(([count]) => count > 0)
  if (first === -1) return '0 s'
  return units
    .slice(first, first + 2)
    .filter(([count]) => count > 0)
    .map(([count, unit]) => `${String(count)} ${unit}`)
    .join(' ')
}

/** One line for an event: that the alert fired, or that it resolved and after how long. */
export function describeEvent(event: EventTimes): string {
  if (event.status === 'firing' || event.endsAt === undefined) return 'Fired'
  return `Resolved after ${formatDuration(Date.parse(event.endsAt) - Date.parse(event.startsAt))}`
}
