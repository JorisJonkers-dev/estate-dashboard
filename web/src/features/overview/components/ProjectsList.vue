<script setup lang="ts">
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { computed, ref } from 'vue'
import { useNow } from '../../../infrastructure/useNow'
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

// Pressing the active filter again shows all.
function pick(value: unknown) {
  filter.value = filters.find((f) => f === value) ?? 'All'
}

const head = 'py-2.5 pr-4 text-left text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
</script>

<template>
  <section aria-labelledby="projects-h">
    <div class="mb-3 flex flex-wrap items-end justify-between gap-4">
      <h2 id="projects-h" class="m-0 text-xl font-semibold">Projects <span class="font-normal text-text-muted">{{ shown.length }}</span></h2>
      <label class="flex min-h-10 w-full items-center gap-2 border-b border-control text-text-muted focus-within:border-accent md:w-72">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-4-4" />
        </svg>
        <span class="sr-only">Search projects</span>
        <input v-model="query" type="search" placeholder="Search" class="min-h-10 grow border-0 bg-transparent font-sans text-base text-text-primary outline-none" />
      </label>
    </div>

    <ToggleGroupRoot
      type="single"
      :model-value="filter"
      aria-label="Filter by state"
      class="flex gap-6 overflow-x-auto border-b border-line md:flex-wrap md:overflow-visible"
      @update:model-value="pick"
    >
      <ToggleGroupItem
        v-for="f in filters"
        :key="f"
        :value="f"
        class="-mb-px min-h-11 shrink-0 cursor-pointer border-0 border-b-2 border-transparent bg-transparent px-0.5 font-sans text-[15px] whitespace-nowrap text-nav transition-colors hover:text-white data-[state=on]:border-accent data-[state=on]:text-white"
      >
        {{ f }} <span class="text-dim">{{ counts[f] }}</span>
      </ToggleGroupItem>
    </ToggleGroupRoot>

    <p v-if="loading" class="my-4 text-text-muted">Loading…</p>
    <p v-else-if="shown.length === 0" class="my-4 text-text-muted">No projects match.</p>
    <table v-else aria-label="Projects" class="w-full border-collapse">
      <thead class="hidden md:table-header-group">
        <tr class="border-b border-line">
          <th scope="col" :class="head">Project</th>
          <th scope="col" :class="head">State</th>
          <th scope="col" :class="head">Version</th>
          <th scope="col" :class="head">Deployed</th>
          <th scope="col" :class="head">Path</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="p in shown"
          :key="p.name"
          class="relative grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 border-b border-line-soft py-3 transition-colors hover:bg-panel md:table-row md:py-0"
        >
          <th scope="row" class="text-left font-medium md:py-3 md:pr-4">
            <!-- The link covers the whole row. -->
            <RouterLink :to="`/projects/${p.name}`" class="text-text-primary no-underline after:absolute after:inset-0 hover:text-text-primary">{{ p.name }}</RouterLink>
          </th>
          <td class="md:py-3 md:pr-4"><StateTag :tone="p.tone">{{ p.state }}</StateTag></td>
          <td class="col-span-2 text-sm text-text-muted md:col-span-1 md:py-3 md:pr-4 md:text-base md:text-text-primary">
            <span v-if="p.version" class="mr-2 font-medium">{{ p.version }}</span>
            <span class="font-mono text-xs text-dim">{{ p.path === 'legacy' ? 'fleet-infra' : (p.pin ?? '') }}</span>
          </td>
          <td class="hidden text-text-muted md:table-cell md:py-3 md:pr-4">
            <time v-if="p.deployedAt" :datetime="p.deployedAt">{{ age(p.deployedAt, now) }} ago</time>
          </td>
          <td class="hidden text-text-muted md:table-cell md:py-3">{{ p.path === 'legacy' ? 'old path' : 'estate path' }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>
