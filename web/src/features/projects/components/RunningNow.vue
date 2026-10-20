<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { Pod, Update } from '../../../infrastructure/api/types.gen'
import { glossary } from '../../../shared/glossary'
import { toneText } from '../../../shared/tone'
import TermHint from '../../../shared/ui/TermHint.vue'
import { runningNow, type ContainerLine, type ProcessGroup } from '../domain/running'

/** What runs now: one table, each Process with its sidecars under it, then the pods still on an older release. */
const props = defineProps<{
  project: string
  release?: string | undefined
  pods: Pod[]
  updates: Update[]
  loading: boolean
  unreadable: boolean
  now: number
}>()
const emit = defineEmits<{ logs: [container: string, version: string | undefined] }>()
const router = useRouter()

const table = computed(() => runningNow(props.pods, props.release, props.updates, props.now))
const pageOf = (line: ContainerLine) => `/containers/${line.repository}`

interface Row {
  key: string
  group: ProcessGroup
  line: ContainerLine
  sidecar: boolean
}
const rowsOf = (groups: ProcessGroup[]): Row[][] =>
  groups.map((g) => [
    { key: g.process, group: g, line: g.main, sidecar: false },
    ...g.sidecars.map((s) => ({ key: `${g.process}/${s.container}`, group: g, line: s, sidecar: true })),
  ])

const head = 'py-2.5 pr-4 text-left text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const button =
  'relative z-10 inline-flex min-h-8 cursor-pointer items-center rounded-md border border-control bg-transparent px-3 font-sans text-sm text-text-primary transition-colors hover:border-accent hover:text-accent max-md:min-h-11'
</script>

<template>
  <section aria-labelledby="running-h">
    <h2 id="running-h" class="m-0 mb-3 text-xl font-semibold">
      <TermHint :text="glossary.runningNow" term>Running now</TermHint>
    </h2>
    <p v-if="loading" class="m-0 text-text-muted">Loading…</p>
    <p v-else-if="unreadable" class="m-0 text-text-muted">Not read: the cluster did not answer.</p>
    <p v-else-if="table.current.length === 0 && table.older.length === 0" class="m-0 text-text-muted">
      No container of {{ project }} is running.
    </p>
    <template v-else>
      <template v-for="(part, n) in [{ groups: table.current }, ...table.older]" :key="n">
        <h3 v-if="'release' in part" class="m-0 mt-6 mb-2 flex items-baseline gap-2.5 text-base font-semibold">
          Still on release {{ part.release }}
          <span v-if="part.stopping" class="rounded-full border border-current px-2 text-xs font-normal text-back"
            >stopping</span
          >
        </h3>
        <table
          :aria-label="'release' in part ? `Still on release ${part.release}` : 'Running now'"
          class="w-full table-fixed border-collapse"
          :class="{ 'opacity-80': 'release' in part }"
        >
          <colgroup class="hidden md:table-column-group">
            <col class="w-[24%]" />
            <col class="w-[12%]" />
            <col class="w-[22%]" />
            <col class="w-[30%]" />
            <col class="w-[12%]" />
          </colgroup>
          <thead v-if="n === 0" class="hidden md:table-header-group">
            <tr class="border-b border-line">
              <th scope="col" :class="head">Process</th>
              <th scope="col" :class="head">Ready</th>
              <th scope="col" :class="head">Container</th>
              <th scope="col" :class="head">Version</th>
              <th scope="col" :class="head"><span class="sr-only">Logs</span></th>
            </tr>
          </thead>
          <tbody
            v-for="rows in rowsOf(part.groups)"
            :key="rows[0]?.key"
            class="border-b border-line"
            :class="{ 'border-t': n > 0 }"
          >
            <tr
              v-for="r in rows"
              :key="r.key"
              class="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 py-2 hover:bg-panel md:table-row md:py-0"
              :class="{ 'max-md:pl-5': r.sidecar }"
              @click="router.push(pageOf(r.line))"
            >
              <th scope="row" class="min-w-0 text-left font-medium md:py-2.5 md:pr-4">
                <span v-if="!r.sidecar" class="flex items-center gap-2">
                  <span
                    class="size-2.5 shrink-0 rounded-full bg-current"
                    :class="toneText[r.group.tone]"
                    aria-hidden="true"
                  />
                  {{ r.group.process }}
                </span>
                <span v-else class="flex items-center gap-1.5 pl-1 font-normal text-text-muted">
                  <svg
                    width="14"
                    height="14"
                    viewBox="0 0 14 14"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    aria-hidden="true"
                  >
                    <path d="M3 0v8h9" />
                  </svg>
                  {{ r.line.container }} <em class="text-xs not-italic text-dim">sidecar</em>
                </span>
              </th>
              <td class="col-start-1 text-sm text-text-muted md:table-cell md:py-2.5 md:pr-4">
                {{ r.sidecar ? r.line.ready : r.group.ready }} of {{ r.sidecar ? r.line.total : r.group.total
                }}<span class="md:hidden"> ready</span>
              </td>
              <td class="col-start-1 min-w-0 truncate md:table-cell md:py-2.5 md:pr-4">
                <RouterLink :to="pageOf(r.line)" class="relative z-10" @click.stop>{{
                  r.line.repository.slice(r.line.repository.lastIndexOf('/') + 1)
                }}</RouterLink>
              </td>
              <td class="col-start-1 flex min-w-0 flex-wrap items-baseline gap-2 md:table-cell md:py-2.5 md:pr-4">
                <span class="font-mono text-sm break-all">{{ r.line.version ?? '—' }}</span>
                <span v-if="r.line.newer" class="ml-2 rounded-full border border-current px-2 text-xs text-warn"
                  >newer ready</span
                >
              </td>
              <td class="col-start-2 row-span-4 row-start-1 self-center md:table-cell md:py-2.5 md:text-right">
                <button type="button" :class="button" @click.stop="emit('logs', r.line.container, r.line.version)">
                  Logs<span class="sr-only"> of {{ r.line.container }}</span>
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </template>
  </section>
</template>
