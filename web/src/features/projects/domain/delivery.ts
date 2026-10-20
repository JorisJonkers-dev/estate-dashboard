import type { HealthTimeline, Release, Source, Unit } from '../../../infrastructure/api/types.gen'
import type { StripBlock } from '../../../shared/strip'
import type { Tone } from '../../../shared/tone'
import { age, shortDigest } from '../../overview/domain/overview'

/** A fact of delivery: what it says, and its colour. */
export interface Fact {
  word: string
  tone: Tone
}

/** Where a Project's delivery stands: its pin, whether Flux fetched and applied it, and how it switches. */
export interface DeliveryFacts {
  pin?: string
  release?: string
  fetched: Fact
  applied: Fact
  after: string[]
  switches: string
  artifact?: string
}

/** Whether a Flux revision names the digest, with or without a tag before it. */
const names = (revision: string | undefined, digest: string) =>
  revision !== undefined && (revision === digest || revision.endsWith(`@${digest}`))

export function deliveryFacts(project: string, sources: Source[], units: Unit[], releases: Release[]): DeliveryFacts {
  const source = sources.find((s) => s.name === `project-${project}`)
  const unit = units.find((u) => u.source === `project-${project}`)
  const fetched: Fact =
    source === undefined
      ? { word: 'nothing pinned', tone: 'idle' }
      : !source.ready
        ? { word: source.reason ?? 'not ready', tone: 'crit' }
        : names(source.revision, source.digest)
          ? { word: 'yes', tone: 'ok' }
          : { word: 'fetching', tone: 'info' }
  const applied: Fact =
    unit === undefined
      ? { word: 'no unit', tone: 'idle' }
      : !unit.ready
        ? { word: unit.reason ?? 'not ready', tone: 'crit' }
        : source !== undefined && names(unit.applied, source.digest)
          ? { word: 'yes', tone: 'ok' }
          : { word: 'applying', tone: 'info' }
  const gated = releases.some((r) => r.namespace === `${project}-system` && r.members.length > 0)
  return {
    ...(source ? { pin: shortDigest(source.digest), artifact: source.url.replace(/^oci:\/\//, '') } : {}),
    ...(unit?.version ? { release: unit.version } : {}),
    fetched,
    applied,
    after: unit?.dependsOn ?? [],
    switches: gated ? 'blue-green, through a canary' : 'continuously, no canary',
  }
}

const words: Record<HealthTimeline['blocks'][number]['state'], string> = {
  ok: 'Healthy',
  warn: 'Degraded',
  crit: 'Down',
  empty: 'No data',
}

/** A health timeline as strip blocks, each naming its time and state. */
export function healthBlocks(timeline: HealthTimeline, now: number): StripBlock[] {
  return timeline.blocks.map((b) => ({
    tone: b.state,
    label: `${age(b.at, now)} ago · ${words[b.state]}${b.detail ? `, ${b.detail}` : ''}`,
  }))
}

/** A check's recent probes as strip blocks, oldest first. */
export function probeBlocks(recent: boolean[]): StripBlock[] {
  return recent.map((ok, i) => ({
    tone: ok ? 'ok' : 'crit',
    label: `${String(recent.length - i)} probes ago · ${ok ? 'answered' : 'failed'}`,
  }))
}
