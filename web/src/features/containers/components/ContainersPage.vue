<script setup lang="ts">
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toneText } from '../../../shared/tone'
import SideDrawer from '../../../shared/ui/SideDrawer.vue'
import ContainerLogs from '../../projects/components/ContainerLogs.vue'
import { useContainers } from '../composables/useContainers'
import {
  counts,
  cycle,
  filters,
  keep,
  shownVersion,
  sorted,
  type ContainerRow,
  type Filter,
  type Sort,
  type SortKey,
} from '../domain/containers'

const { rows, loading, unreadable } = useContainers()
const router = useRouter()

const filter = ref<Filter>('All')
const query = ref('')
const sort = ref<Sort>(null)
const shown = computed(() => sorted(keep(rows.value, filter.value, query.value), sort.value))
const tally = computed(() => counts(rows.value))

function pick(value: unknown) {
  filter.value = filters.find((f) => f === value) ?? 'All'
}
function showAll() {
  filter.value = 'All'
  query.value = ''
}

const columns: { key: SortKey; label: string }[] = [
  { key: 'name', label: 'Container' },
  { key: 'version', label: 'Version' },
  { key: 'projects', label: 'Runs in' },
]
const ariaSort = (key: SortKey) => (sort.value?.key === key ? sort.value.dir : 'none')

const pageOf = (r: ContainerRow) => `/containers/${r.repository}`

// One drawer, for one image's logs.
const logs = ref<ContainerRow | null>(null)
const logsOpen = computed({
  get: () => logs.value !== null,
  set: (v: boolean) => {
    if (!v) logs.value = null
  },
})
const first = computed(
  () => logs.value?.container.instances.find((i) => !i.deleting) ?? logs.value?.container.instances[0],
)

const head = 'py-2.5 pr-4 text-left text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const button =
  'relative z-10 inline-flex min-h-8 cursor-pointer items-center rounded-md border border-control bg-transparent px-3 font-sans text-sm text-text-primary transition-colors hover:border-accent hover:text-accent max-md:min-h-11'
</script>

<template>
  <div class="flex max-w-6xl flex-col pt-8 lg:pt-10">
    <h1 class="m-0 text-3xl font-semibold lg:text-4xl">Containers</h1>
    <p class="m-0 mt-1 text-text-muted">The software each Project runs, at its own version.</p>

    <div class="mt-7 flex flex-wrap items-end gap-x-6 md:flex-nowrap md:border-b md:border-line">
      <ToggleGroupRoot
        type="single"
        :model-value="filter"
        aria-label="Filter containers"
        class="-mx-6 flex w-[calc(100%+3rem)] min-w-0 gap-6 overflow-x-auto border-b border-line px-6 md:mx-0 md:w-auto md:grow md:border-0 md:px-0"
        @update:model-value="pick"
      >
        <ToggleGroupItem
          v-for="f in filters"
          :key="f"
          :value="f"
          class="-mb-px min-h-11 shrink-0 cursor-pointer border-0 border-b-2 border-transparent bg-transparent px-0.5 font-sans text-[15px] whitespace-nowrap text-nav transition-colors hover:text-white data-[state=on]:border-accent data-[state=on]:text-white"
        >
          {{ f }} <span class="text-dim">{{ tally[f] }}</span>
        </ToggleGroupItem>
      </ToggleGroupRoot>
      <label
        class="mt-3 flex min-h-11 w-full items-center gap-2 rounded-md border border-control px-2.5 text-text-muted focus-within:border-accent md:my-1.5 md:min-h-9 md:w-56 md:shrink-0"
      >
        <svg
          width="15"
          height="15"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          aria-hidden="true"
        >
          <circle cx="11" cy="11" r="7" />
          <path d="M20 20l-4-4" />
        </svg>
        <span class="sr-only">Search containers</span>
        <input
          v-model="query"
          type="search"
          placeholder="Search"
          class="min-h-9 min-w-0 grow border-0 bg-transparent font-sans text-[15px] text-text-primary outline-none"
        />
      </label>
    </div>

    <p v-if="loading" class="my-4 text-text-muted">Loading…</p>
    <p v-else-if="unreadable" class="my-4 text-text-muted">Not read: the cluster did not answer.</p>
    <p v-else-if="shown.length === 0" class="my-4 text-text-muted">
      No container matches.
      <button
        type="button"
        class="cursor-pointer border-0 bg-transparent p-0 font-sans text-base text-text-primary underline underline-offset-4 hover:text-accent"
        @click="showAll"
      >
        Show all
      </button>
    </p>
    <table v-else aria-label="Containers" class="w-full table-fixed border-collapse">
      <colgroup class="hidden md:table-column-group">
        <col class="w-[26%]" />
        <col class="w-[30%]" />
        <col class="w-[34%]" />
        <col class="w-[10%]" />
      </colgroup>
      <thead class="hidden md:table-header-group">
        <tr class="border-b border-line">
          <th v-for="c in columns" :key="c.key" scope="col" :class="head" :aria-sort="ariaSort(c.key)">
            <button
              type="button"
              class="inline-flex cursor-pointer items-center gap-1.5 border-0 bg-transparent p-0 font-sans text-xs font-semibold tracking-[0.12em] uppercase"
              :class="sort?.key === c.key ? 'text-text-primary' : 'text-text-muted'"
              @click="sort = cycle(sort, c.key)"
            >
              {{ c.label }}
              <svg
                width="10"
                height="12"
                viewBox="0 0 10 12"
                fill="none"
                aria-hidden="true"
                :class="sort?.key === c.key ? 'text-accent' : 'text-dim'"
                stroke="currentColor"
                stroke-width="1.6"
              >
                <path v-if="sort?.key === c.key && sort.dir === 'ascending'" d="M5 11V1M1 5l4-4 4 4" />
                <path v-else-if="sort?.key === c.key" d="M5 1v10M1 7l4 4 4-4" />
                <path v-else d="M2 4l3-3 3 3M2 8l3 3 3-3" />
              </svg>
            </button>
          </th>
          <th scope="col" :class="head"><span class="sr-only">Logs</span></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="r in shown"
          :key="r.repository"
          class="relative grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 border-b border-line-soft py-3 transition-colors hover:bg-panel md:table-row md:py-0"
          @click="router.push(pageOf(r))"
        >
          <th scope="row" class="min-w-0 truncate text-left font-medium md:py-3 md:pr-4">
            <RouterLink :to="pageOf(r)" class="text-text-primary hover:text-text-primary" @click.stop>{{
              r.name
            }}</RouterLink>
          </th>
          <td class="row-start-2 flex min-w-0 flex-wrap items-baseline gap-2 md:table-cell md:py-3 md:pr-4">
            <span class="font-mono text-sm break-all">{{ shownVersion(r) }}</span>
            <span
              v-if="r.mark"
              class="ml-2 rounded-full border border-current px-2 text-xs whitespace-nowrap"
              :class="toneText[r.mark.tone]"
              >{{ r.mark.text }}</span
            >
          </td>
          <td class="col-span-2 row-start-3 min-w-0 text-sm text-text-muted md:table-cell md:py-3 md:pr-4 md:text-base">
            <template v-for="(p, i) in r.projects" :key="p"
              ><template v-if="i > 0">, </template
              ><RouterLink :to="`/projects/${p}`" class="relative z-10" @click.stop>{{ p }}</RouterLink></template
            >
          </td>
          <td class="row-span-2 row-start-1 col-start-2 self-start md:table-cell md:py-3 md:text-right">
            <button type="button" :class="button" @click.stop="logs = r">
              Logs<span class="sr-only"> of {{ r.name }}</span>
            </button>
          </td>
        </tr>
      </tbody>
    </table>

    <SideDrawer
      v-model:open="logsOpen"
      :title="logs?.name ?? ''"
      :subtitle="[first?.project, logs ? shownVersion(logs) : ''].filter(Boolean).join(' · ')"
    >
      <ContainerLogs v-if="first" :project="first.project" :container="first.container" />
    </SideDrawer>
  </div>
</template>
