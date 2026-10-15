<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import CountBadge from '../../../shared/ui/CountBadge.vue'
import CutLink from '../../../shared/ui/CutLink.vue'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import StateTag from '../../../shared/ui/StateTag.vue'
import { age, shortDigest } from '../../overview/domain/overview'
import { useProject } from '../composables/useProject'
import { links, stageTone, stageWord } from '../domain/project'

const route = useRoute()
const name = computed(() => String(route.params.name))
const { isLoading, row, repository, pin, unit, cluster, conditions, deploys, rollouts, rolloutsLoading } = useProject(name)
const now = useNow()

const legacy = computed(() => row.value?.path === 'legacy')
const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
const label = 'text-[11px] font-semibold tracking-[0.3em] text-text-muted uppercase'
</script>

<template>
  <div class="flex flex-col gap-8 md:gap-10">
    <RouterLink to="/projects" class="-mb-4 text-[15px] no-underline hover:underline md:-mb-6">← All projects</RouterLink>

    <HeadBand :eyebrow="legacy ? 'Old path · fleet-infra' : 'Estate path'" :title="name">
      <template #title><span v-if="row?.version" class="ml-3 align-middle text-2xl font-medium text-text-muted">{{ row.version }}</span></template>
      <StateTag v-if="row" :tone="row.tone">{{ row.state }}</StateTag>
      <span v-else-if="!isLoading">The estate doesn't know this project.</span>
      <template #acts>
        <CutLink v-if="repository" :to="links.repository(repository)" tone="solid">Repository</CutLink>
        <CutLink v-if="repository" :to="links.deployments(repository)">Deployments</CutLink>
        <template v-if="!legacy && pin">
          <CutLink :to="links.workflow(pin.paused ? 'resume' : 'pause')">{{ pin.paused ? 'Resume' : 'Pause' }}</CutLink>
          <CutLink :to="links.workflow('rollback')">Roll back</CutLink>
        </template>
      </template>
      <template v-if="pin" #facts>
        <dl class="m-0 grid grid-cols-2 gap-x-8 gap-y-3 md:flex md:flex-wrap">
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
      fleet-infra deploys {{ name }} for now. Its history shows up here once it moves to the estate path.
    </EmptyPlate>

    <div v-else class="flex flex-col gap-10 md:grid md:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] md:items-start">
      <div class="flex flex-col gap-10">
        <section aria-labelledby="rollouts-h">
          <h2 id="rollouts-h" class="m-0 mb-4 text-2xl font-semibold">Rollouts<CountBadge :count="rollouts.length" said="rollouts" /></h2>
          <p v-if="rolloutsLoading" class="text-text-muted">Reading GitHub…</p>
          <EmptyPlate v-else-if="rollouts.length === 0" title="No rollouts yet" mark="quiet">The first release that composition pins shows up here.</EmptyPlate>
          <ol v-else class="m-0 flex list-none flex-col gap-3 p-0">
            <li v-for="(r, i) in rollouts" :key="r.id" class="rollout cut rise relative px-6 py-4 md:px-8" :style="{ '--i': i }">
              <div class="flex flex-wrap items-baseline gap-x-4 gap-y-1">
                <span class="text-2xl font-semibold">{{ r.version }}</span>
                <time class="text-text-muted" :datetime="r.at" :title="when.format(new Date(r.at))">{{ age(r.at, now) }} ago</time>
                <span class="ml-auto flex flex-wrap gap-4 text-[15px]">
                  <a v-if="repository" :href="links.commit(repository, r.commit)" class="font-mono text-sm">{{ r.commit.slice(0, 7) }}</a>
                  <a v-if="r.run" :href="r.run">Composition run ↗</a>
                </span>
              </div>
              <ol class="m-0 mt-3 flex list-none flex-col gap-1.5 border-l border-line p-0 pl-4">
                <li v-for="s in r.stages" :key="s.at + s.description" class="flex flex-wrap items-baseline gap-x-3 text-[15px]">
                  <span class="w-20 shrink-0 text-xs font-semibold tracking-wide uppercase" :class="toneText[stageTone(s)]">{{ stageWord(s) }}</span>
                  <span class="min-w-0 grow">{{ s.description }}</span>
                  <a v-if="s.log" :href="s.log" class="text-sm">Log ↗</a>
                </li>
              </ol>
            </li>
          </ol>
        </section>

        <section aria-labelledby="pins-h">
          <div class="mb-4 flex items-end justify-between gap-4">
            <h2 id="pins-h" class="m-0 text-2xl font-semibold">Pin history</h2>
            <a :href="links.pinHistory(name)" class="text-[15px]">On GitHub ↗</a>
          </div>
          <p v-if="deploys.length === 0" class="text-text-muted">No commit has moved this pin yet.</p>
          <ol v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="d in deploys" :key="d.commit" class="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 border-b border-line-soft py-3">
              <span class="min-w-0 truncate">{{ d.message }}</span>
              <span class="flex gap-3 text-sm text-text-muted">
                <time :datetime="d.at">{{ age(d.at, now) }} ago</time>
                <a :href="links.commit('JorisJonkers-dev/estate', d.commit)" class="font-mono">{{ d.commit.slice(0, 7) }}</a>
              </span>
            </li>
          </ol>
        </section>
      </div>

      <div class="flex flex-col gap-10">
        <section aria-labelledby="cluster-h">
          <h2 id="cluster-h" class="m-0 mb-4 text-2xl font-semibold">In the cluster</h2>
          <p v-if="cluster.length === 0" class="text-text-muted">Nothing of {{ name }} is running from its pin yet.</p>
          <ul v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="c in cluster" :key="c.kind + c.name" class="flex flex-col gap-0.5 border-b border-line-soft py-3">
              <span class="flex items-baseline justify-between gap-3">
                <span class="font-semibold">{{ c.kind }}</span>
                <span class="font-semibold" :class="toneText[c.tone]">{{ c.word }}</span>
              </span>
              <span class="font-mono text-xs text-text-muted">{{ c.name }}</span>
              <span v-if="c.detail" class="text-sm text-text-muted">{{ c.detail }}</span>
            </li>
          </ul>
        </section>

        <section aria-labelledby="conditions-h">
          <h2 id="conditions-h" class="m-0 mb-4 text-2xl font-semibold">Open issues</h2>
          <p v-if="conditions.length === 0" class="text-text-muted">None.</p>
          <ul v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="c in conditions" :key="c.number" class="border-b border-line-soft py-3">
              <a :href="c.url" class="font-semibold">{{ c.title }}</a>
              <span class="ml-2 text-sm text-text-muted">#{{ c.number }} · {{ age(c.updatedAt, now) }} ago</span>
            </li>
          </ul>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rollout {
  --cut: 1rem;
  background: linear-gradient(150deg, color-mix(in oklab, var(--color-action) 8%, transparent), transparent 55%), var(--color-band);
}
.rollout:first-child {
  background: linear-gradient(150deg, color-mix(in oklab, var(--color-accent) 9%, transparent), transparent 55%), var(--color-band);
}
</style>
