<script setup lang="ts">
import { computed } from 'vue'
import type { Pod, Stream } from '../../../infrastructure/api/types.gen'
import { glossary } from '../../../shared/glossary'
import { toneText } from '../../../shared/tone'
import StateTag from '../../../shared/ui/StateTag.vue'
import StatusMark from '../../../shared/ui/StatusMark.vue'
import TermHint from '../../../shared/ui/TermHint.vue'
import { age } from '../../overview/domain/overview'
import { containerHealth, podHealth, podRole, processes, worst, type Health } from '../domain/pods'

/** What runs now: each Process's pods as Kubernetes reports them, then the Jobs that ran. */
const props = defineProps<{
  project: string
  version?: string | undefined
  pods: Pod[]
  streams: Stream[]
  descriptions: Map<string, string | undefined>
  loading: boolean
  unreadable: boolean
  now: number
}>()
const emit = defineEmits<{ logs: [process: string, streams: Stream[]] }>()

const groups = computed(() => processes(props.pods))
const running = computed(() => groups.value.filter((g) => !g.job))
const runs = computed(() => groups.value.filter((g) => g.job))
const unlabelled = computed(() => props.pods.filter((p) => !p.version).length)
const healthOf = (pods: Pod[]): Health =>
  pods.map((p) => podHealth(p, props.now)).reduce<Health>(worst, { tone: 'ok', word: 'Healthy' })
/** What a Process's pods wrote, newest pod first, for the drawer to switch between. */
const streamsOf = (pods: Pod[]) => {
  const names = new Set(pods.map((p) => p.name))
  return props.streams.filter((s) => names.has(s.pod)).sort((a, b) => (b.first ?? '').localeCompare(a.first ?? ''))
}
const restarts = (p: Pod) => p.containers.reduce((n, c) => n + c.restarts, 0)
/** The probes any of a pod's containers runs, by kind. */
const probesOf = (p: Pod) => {
  const main = p.containers.filter((c) => !c.init)
  return (['readiness', 'liveness', 'startup'] as const).filter((kind) => main.some((c) => c.probes[kind])).join(' ')
}

const button =
  'inline-flex min-h-8 cursor-pointer items-center rounded-md border border-control bg-transparent px-3 font-sans text-sm text-text-primary no-underline transition-colors hover:border-accent hover:text-accent disabled:cursor-not-allowed disabled:opacity-50'
</script>

<template>
  <section aria-labelledby="running-h">
    <h2 id="running-h" class="m-0 mb-3 flex items-center gap-3 text-xl font-semibold">
      <TermHint :text="glossary.runningNow" term>Running now</TermHint>
      <span v-if="version" class="flex items-center gap-2 text-base font-normal text-text-muted"
        ><StatusMark state="live" />{{ version }}</span
      >
    </h2>
    <p v-if="loading" class="m-0 text-text-muted">Loading…</p>
    <p v-else-if="unreadable" class="m-0 text-text-muted">
      The cluster did not answer. Showing nothing rather than guessing.
    </p>
    <p v-else-if="running.length === 0" class="m-0 text-text-muted">No container of {{ project }} is running.</p>
    <template v-else>
      <p v-if="unlabelled > 0" class="m-0 mb-3 text-sm text-warn">
        {{ unlabelled }} of {{ pods.length }} {{ pods.length === 1 ? 'pod carries' : 'pods carry' }} no
        <TermHint :text="glossary.releaseLabel" term>release label</TermHint>, so they can't be tied to a release.
      </p>
      <ul class="m-0 list-none border-t border-line p-0">
        <li v-for="g in running" :key="g.process" class="border-b border-line-soft py-3">
          <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span
              class="size-2.5 shrink-0 self-center rounded-full bg-current"
              :class="toneText[healthOf(g.pods).tone]"
              aria-hidden="true"
            />
            <span class="font-semibold">{{ g.process }}</span>
            <span class="text-sm text-text-muted">{{ g.ready }} of {{ g.pods.length }} ready</span>
            <TermHint :text="glossary.logs"
              ><button
                type="button"
                :class="[button, 'ml-auto shrink-0 self-center']"
                :disabled="streamsOf(g.pods).length === 0"
                @click="emit('logs', g.process, streamsOf(g.pods))"
              >
                Logs
              </button></TermHint
            >
            <span v-if="descriptions.get(g.process)" class="basis-full pl-5.5 text-sm text-text-muted">{{
              descriptions.get(g.process)
            }}</span>
          </div>
          <ul class="m-0 mt-2 ml-5.5 list-none p-0">
            <li v-for="p in g.pods" :key="p.name" class="border-t border-line-soft py-2 first:border-t-0">
              <div class="min-w-0 grow">
                <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <span class="font-mono text-sm" :title="p.name">{{ podRole(p, g.pods) }}</span>
                  <StateTag :tone="podHealth(p, now).tone" :hint="podHealth(p, now).detail">{{
                    podHealth(p, now).word
                  }}</StateTag>
                  <span v-if="p.version" class="text-sm text-text-muted">{{ p.version }}</span>
                  <span v-else class="text-sm text-warn">no release label</span>
                </div>
                <dl class="m-0 mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-sm text-text-muted">
                  <div v-if="p.node" class="flex gap-1">
                    <dt>on</dt>
                    <dd class="m-0 font-mono">{{ p.node }}</dd>
                  </div>
                  <div v-if="p.started" class="flex gap-1">
                    <dt>up</dt>
                    <dd class="m-0">
                      <time :datetime="p.started">{{ age(p.started, now) }}</time>
                    </dd>
                  </div>
                  <div class="flex gap-1">
                    <dt><TermHint :text="glossary.restarts" term>restarts</TermHint></dt>
                    <dd class="m-0" :class="{ 'text-warn': restarts(p) > 0 }">{{ restarts(p) }}</dd>
                  </div>
                  <div class="flex gap-1">
                    <dt><TermHint :text="glossary.probes" term>probes</TermHint></dt>
                    <dd class="m-0" :class="{ 'text-warn': !probesOf(p) }">{{ probesOf(p) || 'none' }}</dd>
                  </div>
                </dl>
                <ul v-if="p.containers.length > 1" class="m-0 mt-1 flex list-none flex-wrap gap-1.5 p-0">
                  <li
                    v-for="c in p.containers"
                    :key="c.name"
                    class="rounded border border-line-soft px-1.5 text-xs"
                    :title="containerHealth(c, now).detail ?? c.image"
                  >
                    <span v-if="c.init" class="text-text-muted">init </span>{{ c.name }}
                    <span :class="toneText[containerHealth(c, now).tone]">{{ containerHealth(c, now).word }}</span>
                  </li>
                </ul>
              </div>
            </li>
          </ul>
        </li>
        <li
          v-for="g in runs"
          :key="'job/' + g.process"
          class="flex flex-wrap items-center gap-3 border-b border-line-soft py-2.5"
        >
          <StatusMark state="past" />
          <span class="font-medium">{{ g.process }}</span>
          <span class="text-sm" :class="toneText[healthOf(g.pods).tone]">{{ healthOf(g.pods).word }}</span>
          <span v-if="g.pods[0]?.started" class="text-sm text-text-muted"
            >started {{ age(g.pods[0].started, now) }} ago</span
          >
          <span v-if="g.pods.length > 1" class="text-sm text-text-muted">{{ g.pods.length }} runs</span>
          <TermHint :text="glossary.logs"
            ><button
              type="button"
              :class="[button, 'ml-auto shrink-0']"
              :disabled="streamsOf(g.pods).length === 0"
              @click="emit('logs', g.process, streamsOf(g.pods))"
            >
              Logs
            </button></TermHint
          >
        </li>
      </ul>
    </template>
  </section>
</template>
