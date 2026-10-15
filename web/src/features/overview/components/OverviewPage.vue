<script setup lang="ts">
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import { computed, ref } from 'vue'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import { useOverview } from '../composables/useOverview'
import { age, filters, passes, type Filter } from '../domain/overview'

const { isLoading, projects, health, needs, unread } = useOverview()
const now = useNow()

const filter = ref<Filter>('All')
const query = ref('')

const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  return projects.value.filter((p) => passes(p, filter.value) && (q === '' || p.name.includes(q)))
})
const counts = computed(() => Object.fromEntries(filters.map((f) => [f, projects.value.filter((p) => passes(p, f)).length])))
const onEstate = computed(() => projects.value.filter((p) => p.path === 'estate').length)
const isExternal = (href: string) => href.startsWith('https://')

/** A single toggle group may be emptied by pressing its one pressed item; the table then shows all. */
function pick(value: unknown) {
  filter.value = filters.find((f) => f === value) ?? 'All'
}

const label = 'text-[13px] font-semibold tracking-[0.08em] uppercase text-text-muted'
</script>

<template>
  <div class="flex flex-col gap-7 md:gap-10">
    <div>
      <h1 class="m-0 text-[28px] font-semibold tracking-[0.01em] md:text-[34px]">Overview</h1>
      <p class="mt-0.5 mb-0 text-sm text-text-muted md:mt-1 md:text-base">
        production · {{ projects.length }} Projects, {{ onEstate }} on the estate path
      </p>
    </div>

    <p v-if="unread.length > 0" role="status" class="m-0 text-[15px] text-warn">
      Not read: <template v-for="(u, i) in unread" :key="u.name">{{ i > 0 ? '; ' : '' }}{{ u.name }} ({{ u.why }})</template>
    </p>

    <dl aria-label="Health by class" class="m-0 grid grid-cols-1 border-t border-line md:grid-cols-4 md:border-b">
      <div
        v-for="h in health"
        :key="h.name"
        class="flex items-baseline justify-between border-b border-line py-3 md:flex-col md:items-start md:justify-start md:gap-1 md:border-b-0 md:px-6 md:py-[18px] md:first:pl-0 md:[&+&]:border-l"
      >
        <dt :class="label">{{ h.name }}</dt>
        <dd class="m-0 flex flex-col items-end md:flex-row md:flex-wrap md:items-baseline md:gap-2.5">
          <span class="text-[17px] font-semibold md:text-[22px]" :class="toneText[h.tone]">{{ h.status }}</span>
          <span class="text-[13px] text-text-muted md:text-[15px]">{{ h.detail }}</span>
        </dd>
      </div>
    </dl>

    <div class="flex flex-col gap-7 md:grid md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)] md:items-start md:gap-12">
      <section aria-labelledby="projects-h">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-4">
          <h2 id="projects-h" class="m-0 text-xl font-semibold">
            Projects <span class="font-normal text-text-muted">{{ shown.length }}</span>
          </h2>
          <label class="flex min-h-11 w-full items-center gap-2 border-b border-[#555555] text-text-muted md:min-h-10 md:w-auto md:min-w-70">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
              <circle cx="11" cy="11" r="7" />
              <path d="M20 20l-4-4" />
            </svg>
            <span class="sr-only">Search projects</span>
            <input v-model="query" type="search" placeholder="Search projects" class="min-h-10 grow border-0 bg-transparent font-sans text-base text-text-primary outline-offset-4" />
          </label>
        </div>
        <ToggleGroupRoot
          type="single"
          :model-value="filter"
          aria-label="Filter by state"
          class="mb-1 flex gap-[18px] overflow-x-auto border-b border-line md:flex-wrap md:gap-6 md:overflow-visible"
          @update:model-value="pick"
        >
          <ToggleGroupItem
            v-for="f in filters"
            :key="f"
            :value="f"
            class="-mb-px min-h-11 shrink-0 cursor-pointer border-0 border-b-2 border-transparent bg-transparent px-0.5 font-sans text-[15px] whitespace-nowrap text-nav data-[state=on]:border-accent data-[state=on]:text-white"
          >
            {{ f }} <span class="text-dim">{{ counts[f] }}</span>
          </ToggleGroupItem>
        </ToggleGroupRoot>

        <p v-if="isLoading" class="my-3 text-text-muted">Reading the estate…</p>
        <p v-else-if="shown.length === 0" class="my-3 text-text-muted">No Project matches.</p>
        <table v-else aria-label="Projects" class="w-full border-collapse">
          <thead class="hidden md:table-header-group">
            <tr>
              <th v-for="h in ['Project', 'State', 'Version', 'Deployed', 'Path']" :key="h" scope="col" class="border-b border-line py-2.5 pr-4 text-left" :class="label">
                {{ h }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="p in shown"
              :key="p.name"
              class="grid grid-cols-[1fr_auto] items-center gap-x-3 border-b border-line-soft py-[11px] [grid-template-areas:'name_state'_'version_state'] md:table-row md:py-0"
            >
              <th scope="row" class="text-left font-medium [grid-area:name] md:border-b md:border-line-soft md:py-3 md:pr-4">{{ p.name }}</th>
              <td class="[grid-area:state] md:border-b md:border-line-soft md:py-3 md:pr-4">
                <span class="inline-block rounded-xs border border-current px-2 py-px text-[13px] font-medium whitespace-nowrap" :class="toneText[p.tone]">{{ p.state }}</span>
              </td>
              <td class="text-sm text-text-muted [grid-area:version] md:border-b md:border-line-soft md:py-3 md:pr-4 md:text-base md:text-text-primary">
                <span v-if="p.version" class="mr-2 font-medium">{{ p.version }}</span>
                <span class="hidden font-mono text-xs text-dim md:inline">{{ p.path === 'legacy' ? 'fleet-infra' : (p.pin ?? '') }}</span>
              </td>
              <td class="hidden text-text-muted md:table-cell md:border-b md:border-line-soft md:py-3 md:pr-4">
                <time v-if="p.deployedAt" :datetime="p.deployedAt">{{ age(p.deployedAt, now) }} ago</time>
              </td>
              <td class="hidden text-text-muted md:table-cell md:border-b md:border-line-soft md:py-3">{{ p.path }}</td>
            </tr>
          </tbody>
        </table>
      </section>

      <section aria-labelledby="needs-h" class="-order-1 md:order-none md:border-l md:border-line md:pl-8">
        <div class="mb-2 flex items-baseline justify-between">
          <h2 id="needs-h" class="m-0 text-xl font-semibold">Needs you</h2>
          <RouterLink to="/alerts" class="text-[15px]">All alerts</RouterLink>
        </div>
        <p v-if="needs.length === 0" class="my-3 text-text-muted">Nothing needs you.</p>
        <ul v-else class="m-0 list-none p-0">
          <li v-for="n in needs" :key="n.key" class="flex flex-col gap-[3px] border-b border-line-soft py-3.5">
            <div class="flex items-center gap-2 text-[13px]">
              <span class="size-2 rounded-full bg-current" :class="toneText[n.tone]" aria-hidden="true" />
              <span class="font-semibold" :class="toneText[n.tone]">{{ n.severity }}</span>
              <span class="text-text-muted">{{ n.class }}</span>
              <time class="ml-auto text-text-muted" :datetime="n.since">{{ age(n.since, now) }}</time>
            </div>
            <a v-if="isExternal(n.href)" :href="n.href" class="font-medium text-text-primary no-underline hover:text-text-primary hover:underline">{{ n.title }}</a>
            <RouterLink v-else :to="n.href" class="font-medium text-text-primary no-underline hover:text-text-primary hover:underline">{{ n.title }}</RouterLink>
            <span class="text-sm text-text-muted">{{ n.where }}</span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
