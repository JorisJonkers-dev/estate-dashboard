<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import type { Stream } from '../../../infrastructure/api/types.gen'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import ActionLink from '../../../shared/ui/ActionLink.vue'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import SideDrawer from '../../../shared/ui/SideDrawer.vue'
import StateTag from '../../../shared/ui/StateTag.vue'
import StatusMark from '../../../shared/ui/StatusMark.vue'
import { age, shortDigest } from '../../overview/domain/overview'
import { useProject } from '../composables/useProject'
import { byProcess, type ProcessLogs } from '../domain/logs'
import { estateRepository, links, stageTone } from '../domain/project'
import { live } from '../domain/releases'
import DeclarationPanel from './DeclarationPanel.vue'
import LogView from './LogView.vue'
import NotesView from './NotesView.vue'

const route = useRoute()
const name = computed(() => String(route.params.name))
const { isLoading, row, repository, pin, unit, cluster, conditions, deploys, releases, streams, rolloutsLoading } = useProject(name)
const now = useNow()

const legacy = computed(() => row.value?.path === 'legacy')
const running = computed(() => live(releases.value))
// The live release's containers, or unlabelled ones until pods carry a version.
const runningProcesses = computed(() => {
  const versioned = running.value ? byProcess(streams.value, running.value.version).filter((p) => !p.migration) : []
  return versioned.length > 0 ? versioned : byProcess(streams.value, undefined).filter((p) => !p.migration)
})
const unversioned = computed(() => runningProcesses.value.length > 0 && !runningProcesses.value[0]?.streams[0]?.version)

const open = ref<number | null>(null)
const toggle = (key: number) => (open.value = open.value === key ? null : key)
const allPins = ref(false)

// One drawer for logs or notes.
type Drawer = { kind: 'logs'; title: string; subtitle: string; streams: Stream[] } | { kind: 'notes'; title: string; subtitle: string; version: string }
const drawer = ref<Drawer | null>(null)
const drawerOpen = computed({
  get: () => drawer.value !== null,
  set: (v: boolean) => {
    if (!v) drawer.value = null
  },
})
function showLogs(p: ProcessLogs, version?: string) {
  const pods = `${String(p.streams.length)} pod${p.streams.length === 1 ? '' : 's'}`
  drawer.value = { kind: 'logs', title: p.process, subtitle: [name.value, version, pods].filter(Boolean).join(' · '), streams: p.streams }
}
function showNotes(version: string) {
  drawer.value = { kind: 'notes', title: `${name.value} ${version}`, subtitle: 'Changelog', version }
}

const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const button =
  'inline-flex min-h-8 cursor-pointer items-center rounded-md border border-control bg-transparent px-3 font-sans text-sm text-text-primary no-underline transition-colors hover:border-accent hover:text-accent'
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand :eyebrow="legacy ? 'Old path, deployed by fleet-infra' : 'Estate path'" :title="name" :tone="row?.tone ?? 'none'">
      <template #title><span v-if="row?.version" class="ml-3 text-2xl font-normal text-text-muted">{{ row.version }}</span></template>
      <StateTag v-if="row" :tone="row.tone">{{ row.state }}</StateTag>
      <span v-else-if="!isLoading">This project isn't in the estate.</span>
      <template #acts>
        <ActionLink v-if="row?.tone === 'info'" :to="`/projects/${name}/release`" primary>Release in flight</ActionLink>
        <ActionLink v-if="repository" :to="links.repository(repository)" :primary="row?.tone !== 'info'">Repository</ActionLink>
        <template v-if="!legacy && pin">
          <ActionLink :to="links.workflow(pin.paused ? 'resume' : 'pause')">{{ pin.paused ? 'Resume' : 'Pause' }}</ActionLink>
          <ActionLink :to="links.workflow('rollback')">Roll back</ActionLink>
        </template>
      </template>
      <template v-if="pin" #facts>
        <dl class="m-0 grid grid-cols-2 gap-x-10 gap-y-3 md:flex md:flex-wrap">
          <div>
            <dt :class="label">Namespace</dt>
            <dd class="m-0 font-mono text-sm">{{ name }}-system</dd>
          </div>
          <div v-if="unit">
            <dt :class="label">Unit</dt>
            <dd class="m-0 font-mono text-sm">{{ unit.name }}<span v-if="unit.dependsOn.length" class="text-text-muted"> after {{ unit.dependsOn.join(', ') }}</span></dd>
          </div>
          <div>
            <dt :class="label">Pin</dt>
            <dd class="m-0 font-mono text-sm"><a :href="links.pin(name)">{{ shortDigest(pin.digest) }}</a></dd>
          </div>
          <div v-if="pin.paused">
            <dt :class="label">Paused</dt>
            <dd class="m-0 text-sm">by {{ pin.paused.by }}<span v-if="pin.paused.reason">: {{ pin.paused.reason }}</span></dd>
          </div>
        </dl>
      </template>
    </HeadBand>

    <EmptyPlate v-if="legacy" title="Still on the old path" mark="quiet">
      fleet-infra deploys {{ name }} for now. History shows up here once it moves to the estate path.
    </EmptyPlate>

    <div v-else class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] lg:items-start lg:gap-12">
      <div class="flex flex-col gap-10">
        <section aria-labelledby="running-h">
          <h2 id="running-h" class="m-0 mb-3 flex items-center gap-3 text-xl font-semibold">
            Running now
            <span v-if="running" class="flex items-center gap-2 text-base font-normal text-text-muted"><StatusMark state="live" />{{ running.version }}</span>
          </h2>
          <p v-if="runningProcesses.length === 0" class="m-0 text-text-muted">No logs from {{ name }} this week.</p>
          <ul v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="p in runningProcesses" :key="p.process" class="flex items-center gap-3 border-b border-line-soft py-2.5">
              <span class="size-2 rounded-full bg-accent" aria-hidden="true" />
              <span class="font-medium">{{ p.process }}</span>
              <span class="text-sm text-text-muted">{{ p.streams.length }} pod{{ p.streams.length === 1 ? '' : 's' }}</span>
              <button type="button" :class="[button, 'ml-auto']" @click="showLogs(p, running?.version)">Logs</button>
            </li>
          </ul>
          <p v-if="unversioned" class="m-0 mt-2 text-sm text-dim">These pods have no version label yet. Older releases may be mixed in.</p>
        </section>

        <section aria-labelledby="releases-h">
          <h2 id="releases-h" class="m-0 mb-3 text-xl font-semibold">Releases</h2>
          <p v-if="rolloutsLoading" class="text-text-muted">Loading…</p>
          <EmptyPlate v-else-if="releases.length === 0" title="No releases yet" mark="quiet">The next release shows up here once composition pins it.</EmptyPlate>
          <ol v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="r in releases" :key="r.key" class="border-b border-line-soft">
              <div class="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
                <StatusMark :state="r.state" />
                <span class="text-lg font-semibold">{{ r.version }}</span>
                <span v-if="r.pins > 1" class="rounded-full border border-control px-2 text-xs text-text-muted" :title="`Pinned ${String(r.pins)} times`">×{{ r.pins }}</span>
                <time class="text-sm text-text-muted" :datetime="r.at" :title="when.format(new Date(r.at))">{{ age(r.at, now) }}</time>
                <span class="ml-auto flex flex-wrap gap-2">
                  <button v-if="repository" type="button" :class="button" @click="showNotes(r.version)">Changelog</button>
                  <a v-if="r.run" :href="r.run" target="_blank" rel="noopener" :class="button">Run<span class="sr-only"> (opens in a new tab)</span></a>
                  <button type="button" :class="button" :aria-expanded="open === r.key" @click="toggle(r.key)">{{ open === r.key ? 'Hide' : 'Details' }}</button>
                </span>
              </div>
              <div v-if="open === r.key" class="mb-4 ml-7 flex flex-col gap-4">
                <ol class="m-0 list-none p-0">
                  <li v-for="s in r.stages" :key="s.at + s.description" class="flex items-baseline gap-2.5 py-0.5 text-sm">
                    <span class="size-1.5 shrink-0 rounded-full bg-current" :class="toneText[stageTone(s)]" aria-hidden="true" />
                    <span class="min-w-0 grow">{{ s.description }}</span>
                    <time class="shrink-0 text-text-muted" :datetime="s.at">{{ age(s.at, now) }}</time>
                  </li>
                </ol>
                <div>
                  <p class="m-0 mb-1.5" :class="label">Containers</p>
                  <p v-if="byProcess(streams, r.version).length === 0" class="m-0 text-sm text-text-muted">None with this version.</p>
                  <ul v-else class="m-0 flex list-none flex-wrap gap-2 p-0">
                    <li v-for="p in byProcess(streams, r.version)" :key="p.process">
                      <button type="button" :class="button" @click="showLogs(p, r.version)">
                        <span v-if="p.migration" class="mr-1.5 text-xs text-text-muted">migration</span>{{ p.process }}
                      </button>
                    </li>
                  </ul>
                </div>
                <p v-if="repository" class="m-0 text-sm text-text-muted">
                  Built from <a :href="links.commit(repository, r.commit)" class="font-mono">{{ r.commit.slice(0, 7) }}</a>
                </p>
              </div>
            </li>
          </ol>
        </section>
      </div>

      <div class="flex flex-col gap-10">
        <DeclarationPanel :project="name" />
        <section aria-labelledby="cluster-h">
          <h2 id="cluster-h" class="m-0 mb-3 text-xl font-semibold">In the cluster</h2>
          <p v-if="cluster.length === 0" class="text-text-muted">Nothing of {{ name }} runs from its pin yet.</p>
          <ul v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="c in cluster" :key="c.kind + c.name" class="flex flex-col gap-0.5 border-b border-line-soft py-3">
              <span class="flex items-baseline justify-between gap-3">
                <span class="font-medium">{{ c.kind }}</span>
                <span class="font-medium" :class="toneText[c.tone]">{{ c.word }}</span>
              </span>
              <span class="font-mono text-xs text-text-muted">{{ c.name }}</span>
              <span v-if="c.detail" class="text-sm text-text-muted">{{ c.detail }}</span>
            </li>
          </ul>
        </section>

        <section aria-labelledby="conditions-h">
          <h2 id="conditions-h" class="m-0 mb-3 text-xl font-semibold">Open issues</h2>
          <p v-if="conditions.length === 0" class="text-text-muted">None.</p>
          <ul v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="c in conditions" :key="c.number" class="border-b border-line-soft py-3">
              <a :href="c.url" class="font-medium">{{ c.title }}</a>
              <span class="ml-2 text-sm text-text-muted">#{{ c.number }}, {{ age(c.updatedAt, now) }} ago</span>
            </li>
          </ul>
        </section>

        <section aria-labelledby="pins-h">
          <div class="mb-3 flex items-end justify-between gap-4">
            <h2 id="pins-h" class="m-0 text-xl font-semibold">Pin history</h2>
            <a :href="links.pinHistory(name)" class="text-[15px]">On GitHub</a>
          </div>
          <p v-if="deploys.length === 0" class="text-text-muted">No commit has moved this pin yet.</p>
          <ol v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="d in allPins ? deploys : deploys.slice(0, 5)" :key="d.commit" class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 border-b border-line-soft py-2.5 text-sm">
              <span class="min-w-0 truncate">{{ d.message }}</span>
              <span class="flex gap-3 text-text-muted">
                <time :datetime="d.at">{{ age(d.at, now) }}</time>
                <a :href="links.commit(estateRepository, d.commit)" class="font-mono">{{ d.commit.slice(0, 7) }}</a>
              </span>
            </li>
          </ol>
          <button v-if="deploys.length > 5" type="button" :class="[button, 'mt-3']" @click="allPins = !allPins">{{ allPins ? 'Show fewer' : `Show all ${String(deploys.length)}` }}</button>
        </section>
      </div>
    </div>

    <SideDrawer v-model:open="drawerOpen" :title="drawer?.title ?? ''" :subtitle="drawer?.subtitle ?? ''">
      <LogView v-if="drawer?.kind === 'logs'" :key="drawer.title + drawer.subtitle" :project="name" :streams="drawer.streams" />
      <NotesView v-else-if="drawer?.kind === 'notes' && repository" :repository="repository" :version="drawer.version" />
    </SideDrawer>
  </div>
</template>
