<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { listReleasesOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useNow } from '../../../infrastructure/useNow'
import { toneText, type Tone } from '../../../shared/tone'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import StatusMark from '../../../shared/ui/StatusMark.vue'
import { age } from '../../overview/domain/overview'

/** How many checks a member must pass before it may promote (the Platform document's analysis). */
const checks = 4

const route = useRoute()
const name = computed(() => String(route.params.name))
const releases = useQuery({ ...listReleasesOptions(), refetchInterval: 10_000 })
const now = useNow()

const release = computed(() => (releases.data.value?.items ?? []).find((r) => r.namespace === `${name.value}-system`))
const switching = computed(() => release.value?.members.some((m) => m.phase && !['Succeeded', 'Initialized', 'Failed'].includes(m.phase)) ?? false)
const allPassed = computed(() => release.value?.members.every((m) => m.iterations >= checks) ?? false)

type Step = { name: string; detail: string; word: string; tone: Tone }
const steps = computed<Step[]>(() => {
  const r = release.value
  if (!r) return []
  const migrated = r.members.some((m) => m.phase && m.phase !== 'Initializing')
  return [
    {
      name: 'Migration up',
      detail: r.migration ? r.migration.identity : 'none declared',
      word: r.migration ? (migrated ? 'Done' : 'Running') : 'Skipped',
      tone: r.migration ? (migrated ? 'ok' : 'info') : 'idle',
    },
    { name: 'Switch', detail: `${String(r.members.length)} member${r.members.length === 1 ? '' : 's'}`, word: switching.value ? 'In progress' : 'Done', tone: switching.value ? 'info' : 'ok' },
  ]
})

const conditions = computed(() => {
  const r = release.value
  if (!r) return []
  return [
    { label: `every primary runs ${r.migration?.testedAgainst ?? 'the tested release'}`, met: r.migration?.testedAgainst !== undefined && r.serving === r.migration.testedAgainst },
    { label: 'no non-transactional changeset', met: r.migration ? !r.migration.nonTransactional : true },
  ]
})
const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand eyebrow="Release" :title="release ? `${name} ${release.serving ?? ''} → ${release.pinned ?? ''}` : name" :tone="switching ? 'info' : 'none'">
      <span v-if="release?.since">Started {{ age(release.since, now) }} ago</span>
      <template #acts>
        <RouterLink :to="`/projects/${name}`" class="inline-flex min-h-10 items-center rounded-md border border-control px-4 text-[15px] font-medium no-underline hover:border-accent">Back to {{ name }}</RouterLink>
      </template>
    </HeadBand>

    <EmptyPlate v-if="!releases.isPending.value && !release" title="Nothing switching" mark="quiet">The Release Gate holds no release for {{ name }}.</EmptyPlate>
    <template v-else-if="release">
      <section aria-labelledby="order-h">
        <h2 id="order-h" class="m-0 mb-3 text-xl font-semibold">Order</h2>
        <ol class="m-0 grid list-none grid-cols-1 border-t border-line p-0 md:grid-cols-2">
          <li v-for="(s, i) in steps" :key="s.name" class="flex flex-col gap-1 border-b border-line py-4 md:pr-6" :class="{ 'md:border-l md:pl-6': i > 0 }">
            <span :class="label">Step {{ i + 1 }}</span>
            <span class="text-lg font-semibold">{{ s.name }}</span>
            <span class="text-text-muted">{{ s.detail }}</span>
            <span class="font-semibold" :class="toneText[s.tone]">{{ s.word }}</span>
          </li>
        </ol>
      </section>

      <div class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] lg:items-start lg:gap-12">
        <section aria-labelledby="barrier-h">
          <div class="mb-3 flex flex-wrap items-baseline justify-between gap-3">
            <h2 id="barrier-h" class="m-0 text-xl font-semibold">Barrier</h2>
            <span class="text-sm text-text-muted">No member promotes until every member has passed.</span>
          </div>
          <ul class="m-0 list-none border-t border-line p-0">
            <li v-for="m in release.members" :key="m.process" class="flex flex-col gap-3 border-b border-line-soft py-4">
              <div class="flex items-center gap-3">
                <StatusMark :state="m.phase === 'Failed' ? 'failed' : m.phase === 'Succeeded' ? 'live' : 'rolling'" />
                <span class="text-lg font-semibold">{{ m.process }}</span>
                <span class="text-sm text-text-muted">{{ m.phase ?? 'not seen yet' }}</span>
              </div>
              <div class="flex items-center gap-3">
                <span class="text-sm text-text-muted">Checks</span>
                <span class="flex gap-1" :aria-label="`${String(m.iterations)} of ${String(checks)} checks passed`">
                  <span v-for="n in checks" :key="n" class="h-1.5 w-8 rounded-full" :class="n <= m.iterations ? 'bg-accent' : 'bg-line'" />
                </span>
                <span class="text-sm" :class="allPassed ? 'text-ok' : 'text-text-muted'">{{ allPassed ? 'May promote' : 'Waiting' }}</span>
              </div>
            </li>
          </ul>
        </section>

        <section aria-labelledby="down-h" class="flex flex-col gap-3 lg:border-l lg:border-line lg:pl-8">
          <h2 id="down-h" class="m-0 text-xl font-semibold">If it's held</h2>
          <p class="m-0 text-text-muted">The down migration only runs when all of these hold:</p>
          <ul class="m-0 flex list-none flex-col gap-2 p-0">
            <li v-for="c in conditions" :key="c.label" class="flex items-baseline gap-2.5">
              <span class="w-4 font-semibold" :class="c.met ? 'text-ok' : 'text-dim'">{{ c.met ? '✓' : '·' }}</span>
              <span>{{ c.label }}</span>
            </li>
          </ul>
          <p class="m-0 text-sm text-text-muted">A promoted member is never undone automatically. The fix is forward.</p>
        </section>
      </div>
    </template>
  </div>
</template>
