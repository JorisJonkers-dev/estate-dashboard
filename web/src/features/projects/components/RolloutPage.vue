<script setup lang="ts">
import { keepPreviousData, useQuery } from '@tanstack/vue-query'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getRolloutWindowOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import StatusStrip from '../../../shared/ui/StatusStrip.vue'
import { age } from '../../overview/domain/overview'
import { flyingInterval, useProject } from '../composables/useProject'
import {
  canaryWords,
  clock,
  howItSwitches,
  containerAt,
  containerTrack,
  ifItFails,
  laneAt,
  lanesOf,
  lastMoment,
  orderAt,
  rolloutState,
  runAt,
  runsOf,
  runTrack,
  windowOf,
} from '../domain/rollout'
import TimeTrack from './TimeTrack.vue'

const route = useRoute()
const name = computed(() => String(route.params.name))
const { releases, rolloutsLoading, pods, gate, gates, pin, unit } = useProject(name)
// A release in flight is followed live, so its end moves at the pace its reads do.
const now = useNow(flyingInterval)

const index = computed(() => {
  const asked = route.query.version
  const i = releases.value.findIndex((r) => r.version === asked)
  return i >= 0 ? i : 0
})
const view = computed(() => releases.value[index.value])
const from = computed(() => releases.value[index.value + 1]?.version)
const span = computed(() => (view.value ? windowOf(view.value, now.value) : undefined))
const rolling = computed(() => view.value?.state === 'rolling')
/** Whether the Canaries switch the release shown: the newest, once Flux applied it. */
const current = computed(() => index.value === 0 && unit.value?.version === view.value?.version)

// While a release is in flight its window grows with every read; the last one shows until the next answers.
const window = useQuery(
  computed(() => ({
    ...getRolloutWindowOptions({
      path: { project: name.value },
      query: { from: span.value?.from ?? '', to: span.value?.to ?? '' },
    }),
    enabled: span.value !== undefined,
    refetchInterval: rolling.value ? flyingInterval : false,
    placeholderData: rolling.value ? keepPreviousData : undefined,
    retry: false,
  })),
)

const failedAt = computed(() => {
  const failed = view.value?.stages.find((s) => s.state === 'failure' || s.state === 'error')
  return failed && span.value ? Math.round((Date.parse(failed.at) - span.value.t0) / 1000) : undefined
})
const lanes = computed(() =>
  view.value && span.value && window.data.value
    ? lanesOf({
        view: view.value,
        from: from.value,
        window: window.data.value,
        pods: pods.value,
        t0: span.value.t0,
        failedAt: failedAt.value,
        members: gates.value.flatMap((g) => g.members),
        current: current.value,
      })
    : [],
)
const runs = computed(() => (span.value && window.data.value ? runsOf(window.data.value, span.value.t0) : []))
const end = computed(() =>
  rolling.value ? (span.value?.end ?? 1) : lastMoment(lanes.value, span.value?.end ?? 1, runs.value),
)

// Replay: a finished release starts at its end, a release in flight follows live until scrubbed.
const t = ref<number | null>(null)
const following = computed(() => rolling.value && t.value === null)
const at = computed(() => (t.value === null ? end.value : Math.min(t.value, end.value)))
const playing = ref(false)
let timer: ReturnType<typeof setInterval> | undefined
function stop() {
  playing.value = false
  if (timer !== undefined) clearInterval(timer)
  timer = undefined
}
function play() {
  if (playing.value) {
    stop()
    return
  }
  if (at.value >= end.value) t.value = 0
  playing.value = true
  timer = setInterval(() => {
    const next = at.value + 1
    if (next >= end.value) {
      t.value = rolling.value ? null : end.value
      stop()
    } else t.value = next
  }, 100)
}
function restart() {
  stop()
  t.value = 0
}
/** Follows the release live again: where it is in flight, the page keeps up with it on its own. */
function follow() {
  stop()
  t.value = null
}
function scrub(event: Event) {
  stop()
  const value = Number((event.target as HTMLInputElement).value)
  t.value = rolling.value && value >= end.value ? null : value
}
// Opening a release follows it live while it is in flight, and shows a finished one at its end.
watch(() => route.fullPath, follow)
onBeforeUnmount(stop)

const order = computed(() => (view.value && span.value ? orderAt(view.value, gate.value, span.value.t0, at.value) : []))
const undo = computed(() => ifItFails(name.value, from.value, gate.value))
const state = computed(() => (view.value ? rolloutState(view.value) : undefined))
const switching = computed(() => howItSwitches(lanes.value))
const podTone: Record<string, string> = { ...toneText, gone: 'text-dim' }

const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const head = 'py-2.5 pr-4 text-left text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const icon =
  'inline-flex size-10 shrink-0 cursor-pointer items-center justify-center rounded-md border border-control bg-transparent text-text-primary transition-colors hover:border-accent hover:text-accent max-md:size-11'
const stepIcon: Record<string, string> = {
  done: 'text-ok',
  running: 'text-info',
  waiting: 'text-dim',
  failed: 'text-crit',
}
</script>

<template>
  <div class="flex flex-col gap-10">
    <header class="-mx-6 border-b border-line bg-band px-6 pt-8 pb-7 lg:-mx-10 lg:px-10 lg:pt-10">
      <RouterLink :to="`/projects/${name}`" class="text-sm text-text-muted">{{ name }} · Rollout</RouterLink>
      <div class="mt-1.5 flex flex-wrap items-end justify-between gap-6">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-3">
            <h1 class="m-0 text-3xl leading-tight font-semibold lg:text-4xl">
              {{ name }}
              <span v-if="view" class="ml-2 font-mono text-2xl font-normal text-text-muted"
                ><template v-if="from">{{ from }} <span aria-label="to">→</span> </template>{{ view.version }}</span
              >
            </h1>
            <span
              v-if="state"
              class="rounded-full border border-current px-2.5 py-0.5 text-sm"
              :class="toneText[state.tone]"
              >{{ state.word }}</span
            >
            <span v-if="following" class="flex items-center gap-1.5 text-sm text-accent"
              ><span class="size-2 animate-pulse rounded-full bg-accent" aria-hidden="true" />Live</span
            >
            <button
              v-else-if="rolling"
              type="button"
              class="flex cursor-pointer items-center gap-1.5 rounded-full border border-accent bg-transparent px-2.5 py-0.5 font-sans text-sm text-accent"
              @click="follow"
            >
              <span class="size-2 rounded-full bg-accent" aria-hidden="true" />Back to live
            </button>
          </div>
          <dl v-if="view" class="m-0 mt-5 flex flex-wrap gap-x-10 gap-y-3">
            <div>
              <dt :class="label">Started</dt>
              <dd class="m-0 text-sm">
                <time :datetime="view.at">{{ age(view.at, now) }} ago</time>
              </dd>
            </div>
            <div>
              <dt :class="label">Switches</dt>
              <dd class="m-0 text-sm">
                {{
                  gates.some((g) => g.members.length > 0) ? 'blue-green, through a canary' : 'continuously, no canary'
                }}
              </dd>
            </div>
            <div v-if="pin">
              <dt :class="label">Pin</dt>
              <dd class="m-0 font-mono text-sm">{{ pin.digest.replace(/^sha256:/, '').slice(0, 12) }}</dd>
            </div>
          </dl>
        </div>
        <div v-if="view" role="group" aria-label="Replay" class="flex w-full items-center gap-2 md:w-[26rem]">
          <button type="button" :class="icon" aria-label="Back to the start" @click="restart">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor" aria-hidden="true">
              <path d="M2 1h2v12H2zM13 1v12L5 7z" />
            </svg>
          </button>
          <button type="button" :class="icon" :aria-label="playing ? 'Pause' : 'Play'" @click="play">
            <svg v-if="playing" width="14" height="14" viewBox="0 0 14 14" fill="currentColor" aria-hidden="true">
              <path d="M3 1h3v12H3zM8 1h3v12H8z" />
            </svg>
            <svg v-else width="14" height="14" viewBox="0 0 14 14" fill="currentColor" aria-hidden="true">
              <path d="M3 1l10 6-10 6z" />
            </svg>
          </button>
          <label class="sr-only" for="scrub">Time in the rollout</label>
          <input
            id="scrub"
            type="range"
            min="0"
            :max="end"
            step="1"
            :value="at"
            class="min-w-0 grow accent-[var(--color-accent)]"
            @input="scrub"
          />
          <time class="shrink-0 font-mono text-sm text-text-muted">{{ clock(at) }} of {{ clock(end) }}</time>
        </div>
      </div>
    </header>

    <p v-if="rolloutsLoading" class="m-0 text-text-muted">Loading…</p>
    <p v-else-if="!view" class="m-0 text-text-muted">{{ name }} has no release to show yet.</p>
    <div
      v-else
      class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,1.7fr)_minmax(0,1fr)] lg:items-start lg:gap-12"
    >
      <div class="flex min-w-0 flex-col gap-10">
        <section aria-labelledby="lanes-h" class="min-w-0">
          <h2 id="lanes-h" class="m-0 mb-3 text-xl font-semibold">Containers</h2>
          <p v-if="window.isPending.value" class="m-0 text-text-muted">Loading…</p>
          <p v-else-if="window.isError.value" class="m-0 text-text-muted">Not read: Prometheus did not answer.</p>
          <p v-else-if="lanes.length === 0" class="m-0 text-text-muted">
            No pod of {{ name }} ran in this release's window.
          </p>
          <template v-else>
            <table aria-label="Containers" class="w-full table-fixed border-collapse">
              <colgroup class="hidden md:table-column-group">
                <col class="w-[20%]" />
                <col class="w-[18%]" />
                <col class="w-[18%]" />
                <col class="w-[22%]" />
                <col class="w-[22%]" />
              </colgroup>
              <thead class="hidden md:table-header-group">
                <tr class="border-b border-line">
                  <th scope="col" :class="head">Process</th>
                  <th scope="col" :class="head">Old</th>
                  <th scope="col" :class="head">New</th>
                  <th scope="col" :class="head">Health checks</th>
                  <th scope="col" :class="head">Traffic</th>
                </tr>
              </thead>
              <tbody v-for="l in lanes" :key="l.process" class="border-b border-line">
                <tr class="grid grid-cols-2 gap-x-4 gap-y-2 py-3 md:table-row md:py-0">
                  <th scope="row" class="col-span-2 text-left font-semibold md:py-3 md:pr-4">
                    {{ l.process }}
                    <em
                      v-if="canaryWords(l)"
                      class="block text-xs font-normal not-italic"
                      :class="l.canary?.phase === 'Failed' ? 'text-crit' : 'text-text-muted'"
                      >{{ canaryWords(l) }}</em
                    >
                  </th>
                  <td class="md:py-3 md:pr-4">
                    <span class="block font-mono text-sm break-all">{{ l.oldVersion ?? '—' }}</span>
                    <span class="text-sm" :class="podTone[laneAt(l, at).old.tone]">{{ laneAt(l, at).old.word }}</span>
                  </td>
                  <td class="md:py-3 md:pr-4">
                    <span class="block font-mono text-sm break-all">{{ l.newVersion ?? '—' }}</span>
                    <span class="text-sm" :class="podTone[laneAt(l, at).new.tone]">{{ laneAt(l, at).new.word }}</span>
                  </td>
                  <td class="col-span-2 md:py-3 md:pr-4">
                    <StatusStrip :blocks="laneAt(l, at).blocks" :label="`Health checks of the new ${l.process} pod`" />
                  </td>
                  <td class="col-span-2 md:py-3">
                    <div
                      role="img"
                      :aria-label="`Traffic: new ${String(laneAt(l, at).share)}%, old ${String(100 - laneAt(l, at).share)}%`"
                      class="flex h-2.5 w-full overflow-hidden rounded-sm bg-line"
                    >
                      <span
                        class="h-full transition-[width]"
                        :class="laneAt(l, at).failed ? 'bg-crit' : 'bg-accent'"
                        :style="{ width: `${String(laneAt(l, at).share)}%` }"
                      />
                      <span
                        class="h-full bg-dim"
                        :class="{ 'opacity-30': laneAt(l, at).old.word === 'Stopped' }"
                        :style="{ width: `${String(100 - laneAt(l, at).share)}%` }"
                      />
                    </div>
                    <span class="mt-1 flex justify-between text-xs text-text-muted"
                      ><span>new {{ laneAt(l, at).share }}%</span
                      ><span>old {{ 100 - laneAt(l, at).share }}%</span></span
                    >
                    <small class="text-xs" :class="laneAt(l, at).failed ? 'text-crit' : 'text-text-muted'">{{
                      laneAt(l, at).next
                    }}</small>
                  </td>
                </tr>
                <tr
                  v-for="c in l.containers"
                  :key="c.name"
                  class="grid grid-cols-2 gap-x-4 gap-y-1.5 pb-3 pl-5 text-sm text-text-muted md:table-row md:pb-0 md:pl-0"
                >
                  <th scope="row" class="col-span-2 text-left font-normal md:py-2 md:pr-4">
                    <span class="flex items-start gap-1.5 md:pl-1">
                      <svg
                        width="14"
                        height="14"
                        viewBox="0 0 14 14"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.5"
                        aria-hidden="true"
                        class="mt-0.5 shrink-0"
                      >
                        <path d="M3 0v8h9" />
                      </svg>
                      <span class="min-w-0">
                        <span class="break-words">{{ c.name }}</span>
                        <em class="block text-xs not-italic text-dim">{{
                          c.kind === 'init' ? 'init, runs once' : 'sidecar'
                        }}</em>
                      </span>
                    </span>
                  </th>
                  <td class="font-mono break-all md:py-2 md:pr-4">{{ c.oldVersion ?? '—' }}</td>
                  <td class="font-mono break-all md:py-2 md:pr-4">{{ c.newVersion ?? '—' }}</td>
                  <td class="col-span-2 md:py-2 md:pr-4">
                    <TimeTrack
                      v-if="c.changes.length > 0"
                      :track="containerTrack(c, at)"
                      :length="end"
                      :label="`${c.name}: ${containerAt(c, at).word}`"
                    />
                    <template v-else>runs with its pod</template>
                  </td>
                  <td class="col-span-2 md:py-2">
                    <span v-if="c.changes.length > 0" class="block" :class="podTone[containerAt(c, at).tone]">{{
                      containerAt(c, at).word
                    }}</span>
                    <small class="text-xs">{{
                      c.kind === 'init' ? 'runs before the Process starts' : 'takes no traffic'
                    }}</small>
                  </td>
                </tr>
              </tbody>
            </table>
            <p class="m-0 mt-3 text-sm text-text-muted">{{ switching }}</p>
          </template>
        </section>

        <section aria-labelledby="runs-h" class="min-w-0">
          <h2 id="runs-h" class="m-0 mb-3 text-xl font-semibold">Runs</h2>
          <p v-if="window.isPending.value" class="m-0 text-text-muted">Loading…</p>
          <p v-else-if="window.isError.value" class="m-0 text-text-muted">Not read: Prometheus did not answer.</p>
          <p v-else-if="runs.length === 0" class="m-0 text-text-muted">
            No Job ran in {{ name }}'s namespace in this release's window.
          </p>
          <template v-else>
            <p class="m-0 mb-2 text-sm text-text-muted">
              One-shot work, each pod a Job ran: it starts, does its work and ends. It has no health checks and takes no
              traffic, so once it ends it is done and gone, not down.
            </p>
            <ul aria-label="Runs" class="m-0 list-none border-t border-line p-0">
              <li
                v-for="g in runs"
                :key="`${g.kind}/${g.name}`"
                class="grid grid-cols-[minmax(0,1fr)] gap-y-1.5 border-b border-line py-3 md:grid-cols-[24%_minmax(0,1fr)_12rem] md:items-center md:gap-x-4"
              >
                <span class="min-w-0">
                  <span class="block font-semibold break-words">{{ g.name }}</span>
                  <em class="text-xs not-italic text-dim">{{ g.kind }}</em>
                </span>
                <TimeTrack
                  :track="runTrack(g, at)"
                  :length="end"
                  :label="`${g.name}: ${runAt(g, at).word}, ${runAt(g, at).detail}`"
                />
                <span class="text-sm">
                  <span :class="podTone[runAt(g, at).tone]">{{ runAt(g, at).word }}</span>
                  <small class="block text-xs text-text-muted">{{ runAt(g, at).detail }}</small>
                </span>
              </li>
            </ul>
            <p class="m-0 mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-text-muted" aria-hidden="true">
              <span class="flex items-center gap-1.5"
                ><span
                  class="inline-block h-2.5 w-5 rounded-sm bg-info/60 [background-image:repeating-linear-gradient(-45deg,transparent_0_3px,rgb(0_0_0/0.35)_3px_6px)]"
                />at work</span
              >
              <span class="flex items-center gap-1.5"><span class="text-ok">✓</span>succeeded</span>
              <span class="flex items-center gap-1.5"><span class="text-crit">✕</span>failed</span>
              <span class="flex items-center gap-1.5"><span class="text-dim">•</span>gone, how it ended unknown</span>
            </p>
          </template>
        </section>
      </div>

      <aside class="flex min-w-0 flex-col gap-10">
        <section aria-labelledby="order-h">
          <h2 id="order-h" class="m-0 mb-3 text-xl font-semibold">Order</h2>
          <ol class="m-0 list-none border-l border-line p-0 pl-5">
            <li v-for="s in order" :key="s.at + s.title" class="relative pb-4">
              <span
                class="absolute top-1 -left-[1.6rem] flex size-3 items-center justify-center rounded-full bg-surface-dark"
                :class="stepIcon[s.state]"
              >
                <svg
                  v-if="s.conditions"
                  width="12"
                  height="12"
                  viewBox="0 0 16 16"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  aria-hidden="true"
                >
                  <path d="M3 2v12M13 2v12M3 6h10M3 10h10" />
                </svg>
                <span
                  v-else
                  class="size-2.5 rounded-full"
                  :class="s.state === 'waiting' ? 'border border-current' : 'bg-current'"
                  aria-hidden="true"
                />
              </span>
              <span class="flex items-baseline justify-between gap-3">
                <span :class="{ 'text-text-muted': s.state === 'waiting', 'text-crit': s.state === 'failed' }">{{
                  s.title
                }}</span>
                <time class="shrink-0 font-mono text-xs text-text-muted">{{ clock(s.at) }}</time>
              </span>
              <span class="sr-only">{{ s.state }}</span>
              <ul
                v-if="s.conditions"
                aria-label="What the Release Gate checked"
                class="m-0 mt-1.5 list-none p-0 text-sm"
              >
                <li
                  v-for="c in s.conditions"
                  :key="c.label"
                  class="flex gap-1.5"
                  :class="c.met ? 'text-ok' : 'text-text-muted'"
                >
                  <span aria-hidden="true">{{ c.met ? '✓' : '·' }}</span
                  >{{ c.label }}<span class="sr-only">{{ c.met ? ', met' : ', not met' }}</span>
                </li>
              </ul>
            </li>
          </ol>
        </section>
        <section aria-labelledby="undo-h">
          <h2 id="undo-h" class="m-0 mb-3 text-xl font-semibold">If it fails</h2>
          <p class="m-0 text-text-muted">{{ undo.text }}</p>
          <ul v-if="undo.conditions.length" class="m-0 mt-2 pl-5 text-sm text-text-muted">
            <li v-for="c in undo.conditions" :key="c">{{ c }}</li>
          </ul>
        </section>
      </aside>
    </div>
  </div>
</template>
