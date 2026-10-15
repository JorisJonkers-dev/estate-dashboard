<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import ActionLink from '../../../shared/ui/ActionLink.vue'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import StateTag from '../../../shared/ui/StateTag.vue'
import { age, shortDigest } from '../../overview/domain/overview'
import { useProject } from '../composables/useProject'
import { estateRepository, links, stageTone, stageWord } from '../domain/project'
import LogsPanel from './LogsPanel.vue'

const route = useRoute()
const name = computed(() => String(route.params.name))
const { isLoading, row, repository, pin, unit, cluster, conditions, deploys, rollouts, rolloutsLoading } = useProject(name)
const now = useNow()

const legacy = computed(() => row.value?.path === 'legacy')
const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand :eyebrow="legacy ? 'Old path, deployed by fleet-infra' : 'Estate path'" :title="name" :tone="row?.tone ?? 'none'">
      <template #title><span v-if="row?.version" class="ml-3 text-2xl font-normal text-text-muted">{{ row.version }}</span></template>
      <StateTag v-if="row" :tone="row.tone">{{ row.state }}</StateTag>
      <span v-else-if="!isLoading">This project isn't in the estate.</span>
      <template #acts>
        <ActionLink v-if="repository" :to="links.repository(repository)" primary>Repository</ActionLink>
        <ActionLink v-if="repository" :to="links.deployments(repository)">Deployments</ActionLink>
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
      fleet-infra still deploys {{ name }}. Its history shows here once it moves to the estate path.
    </EmptyPlate>

    <div v-else class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] lg:items-start lg:gap-12">
      <div class="flex flex-col gap-10">
        <section aria-labelledby="rollouts-h">
          <h2 id="rollouts-h" class="m-0 mb-3 text-xl font-semibold">Rollouts <span class="font-normal text-text-muted">{{ rollouts.length }}</span></h2>
          <p v-if="rolloutsLoading" class="text-text-muted">Loading…</p>
          <EmptyPlate v-else-if="rollouts.length === 0" title="No rollouts yet" mark="quiet">The next release shows here once composition pins it.</EmptyPlate>
          <ol v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="(r, i) in rollouts" :key="r.id" class="rise border-b border-line-soft py-4" :style="{ '--i': i }">
              <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1">
                <span class="text-lg font-semibold">{{ r.version }}</span>
                <time class="text-text-muted" :datetime="r.at" :title="when.format(new Date(r.at))">{{ age(r.at, now) }} ago</time>
                <span class="ml-auto flex flex-wrap gap-4 text-[15px]">
                  <a v-if="repository" :href="links.commit(repository, r.commit)" class="font-mono text-sm">{{ r.commit.slice(0, 7) }}</a>
                  <a v-if="r.run" :href="r.run">Composition run</a>
                </span>
              </div>
              <ol class="m-0 mt-2 list-none p-0">
                <li v-for="s in r.stages" :key="s.at + s.description" class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-3 py-0.5 text-[15px]">
                  <span class="flex w-24 items-center gap-2 text-xs font-semibold uppercase" :class="toneText[stageTone(s)]">
                    <span class="size-1.5 rounded-full bg-current" aria-hidden="true" />{{ stageWord(s) }}
                  </span>
                  <span class="min-w-0">{{ s.description }}</span>
                  <a v-if="s.log" :href="s.log" class="text-sm">Log</a>
                </li>
              </ol>
            </li>
          </ol>
        </section>

        <LogsPanel :project="name" />

        <section aria-labelledby="pins-h">
          <div class="mb-3 flex items-end justify-between gap-4">
            <h2 id="pins-h" class="m-0 text-xl font-semibold">Pin history</h2>
            <a :href="links.pinHistory(name)" class="text-[15px]">On GitHub</a>
          </div>
          <p v-if="deploys.length === 0" class="text-text-muted">No commit has moved this pin yet.</p>
          <ol v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="d in deploys" :key="d.commit" class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 border-b border-line-soft py-3">
              <span class="min-w-0 truncate">{{ d.message }}</span>
              <span class="flex gap-3 text-sm text-text-muted">
                <time :datetime="d.at">{{ age(d.at, now) }} ago</time>
                <a :href="links.commit(estateRepository, d.commit)" class="font-mono">{{ d.commit.slice(0, 7) }}</a>
              </span>
            </li>
          </ol>
        </section>
      </div>

      <div class="flex flex-col gap-10">
        <section aria-labelledby="cluster-h">
          <h2 id="cluster-h" class="m-0 mb-3 text-xl font-semibold">In the cluster</h2>
          <p v-if="cluster.length === 0" class="text-text-muted">Nothing runs from its pin yet.</p>
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
      </div>
    </div>

    <RouterLink to="/projects" class="text-[15px]">All projects</RouterLink>
  </div>
</template>
