<script setup lang="ts">
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { computed, ref } from 'vue'
import { useNow } from '../../../infrastructure/useNow'
import CountBadge from '../../../shared/ui/CountBadge.vue'
import CutRow from '../../../shared/ui/CutRow.vue'
import StateTag from '../../../shared/ui/StateTag.vue'
import { age, filters, passes, type Filter, type ProjectRow } from '../domain/overview'

const { projects, loading } = defineProps<{ projects: ProjectRow[]; loading?: boolean }>()
const now = useNow()

const filter = ref<Filter>('All')
const query = ref('')
const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return projects.filter((p) => passes(p, filter.value) && (q === '' || p.name.includes(q)))
})
const counts = computed(() => Object.fromEntries(filters.map((f) => [f, projects.filter((p) => passes(p, f)).length])))

/** Pressing the one pressed filter empties a single toggle group; the list then shows all. */
function pick(value: unknown) {
  filter.value = filters.find((f) => f === value) ?? 'All'
}
</script>

<template>
  <section aria-labelledby="projects-h">
    <div class="mb-4 flex flex-wrap items-end justify-between gap-4">
      <h2 id="projects-h" class="m-0 text-2xl font-semibold">Projects<CountBadge :count="shown.length" said="shown" /></h2>
      <label class="flex min-h-11 w-full items-center gap-2 border-b border-[#555555] text-text-muted focus-within:border-accent md:w-72">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-4-4" />
        </svg>
        <span class="sr-only">Search projects</span>
        <input v-model="query" type="search" placeholder="Search projects" class="min-h-10 grow border-0 bg-transparent font-sans text-base text-text-primary outline-none" />
      </label>
    </div>

    <ToggleGroupRoot
      type="single"
      :model-value="filter"
      aria-label="Filter by state"
      class="mb-4 flex gap-1.5 overflow-x-auto pb-1 md:flex-wrap md:overflow-visible"
      @update:model-value="pick"
    >
      <ToggleGroupItem
        v-for="f in filters"
        :key="f"
        :value="f"
        class="filter cut min-h-10 shrink-0 cursor-pointer border-0 px-4 font-sans text-[15px] whitespace-nowrap"
      >
        {{ f }} <span class="count">{{ counts[f] }}</span>
      </ToggleGroupItem>
    </ToggleGroupRoot>

    <p v-if="loading" class="my-3 text-text-muted">Reading the estate…</p>
    <p v-else-if="shown.length === 0" class="my-3 text-text-muted">No Project matches.</p>
    <ul v-else aria-label="Projects" class="m-0 flex list-none flex-col gap-1.5 p-0">
      <li v-for="(p, i) in shown" :key="p.name" class="rise" :style="{ '--i': Math.min(i, 12) }">
        <CutRow :to="`/projects/${p.name}`" :label="`${p.name}, ${p.state}`" class="grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[minmax(0,1fr)_auto]">
          <div class="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-0.5 md:grid-cols-[minmax(0,1.3fr)_minmax(0,1.5fr)_minmax(0,1.3fr)_minmax(0,0.9fr)]">
            <span class="min-w-0">
              <span class="block truncate text-lg font-semibold">{{ p.name }}</span>
              <span class="block text-[11px] font-semibold tracking-[0.2em] text-text-muted uppercase">{{ p.path === 'legacy' ? 'old path' : 'estate path' }}</span>
            </span>
            <span class="justify-self-end md:justify-self-start"><StateTag :tone="p.tone">{{ p.state }}</StateTag></span>
            <span class="col-span-2 flex items-baseline gap-2 text-sm text-text-muted md:col-span-1 md:text-base md:text-text-primary">
              <span v-if="p.version" class="font-semibold">{{ p.version }}</span>
              <span class="font-mono text-xs text-dim">{{ p.path === 'legacy' ? 'fleet-infra' : (p.pin ?? '') }}</span>
            </span>
            <span class="hidden text-text-muted md:block">
              <time v-if="p.deployedAt" :datetime="p.deployedAt">{{ age(p.deployedAt, now) }} ago</time>
            </span>
          </div>
        </CutRow>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.filter {
  --cut: 0.55rem;
  color: var(--color-nav);
  background: color-mix(in oklab, var(--color-text-primary) 6%, transparent);
  transition:
    background-color 220ms ease,
    color 220ms ease;
}
.filter:hover {
  color: #ffffff;
  background: color-mix(in oklab, var(--color-text-primary) 12%, transparent);
}
.filter[data-state='on'] {
  color: #1c1c1c;
  font-weight: 600;
  background: var(--color-accent);
}
.count {
  margin-left: 0.25rem;
  opacity: 0.7;
}
</style>
