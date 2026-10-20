<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listContainerHistoryOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { ContainerInstance } from '../../../infrastructure/api/types.gen'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import GitHubLink from '../../../shared/ui/GitHubLink.vue'
import SideDrawer from '../../../shared/ui/SideDrawer.vue'
import { age } from '../../overview/domain/overview'
import ContainerLogs from '../../projects/components/ContainerLogs.vue'
import { useContainers } from '../composables/useContainers'
import {
  short,
  shownVersion,
  sourceRepository,
  sourceWords,
  spread,
  tagOf,
  versionsRunning,
} from '../domain/containers'

const route = useRoute()
const router = useRouter()
const now = useNow()
const repository = computed(() => String(route.params.repository))
const { rows, loading, unreadable, updatesUnreadable, repositoryOf } = useContainers()
const row = computed(() => rows.value.find((r) => r.repository === repository.value))
const c = computed(() => row.value?.container)
const source = computed(() => (c.value ? sourceRepository(c.value, repositoryOf) : undefined))
const pinned = computed(() => c.value?.instances.find((i) => i.lockDigest !== undefined))
const drifting = computed(() => c.value?.instances.find((i) => i.drift))

const history = useQuery(
  computed(() => ({ ...listContainerHistoryOptions({ query: { repository: repository.value } }), retry: false })),
)

const role = (i: ContainerInstance) =>
  i.sidecar ? `${i.container}, beside ${i.process}` : i.init ? `${i.container}, before ${i.process}` : i.process

const logs = ref<ContainerInstance | null>(null)
const logsOpen = computed({
  get: () => logs.value !== null,
  set: (v: boolean) => {
    if (!v) logs.value = null
  },
})

const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const head = 'py-2.5 pr-4 text-left text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const button =
  'relative z-10 inline-flex min-h-8 cursor-pointer items-center rounded-md border border-control bg-transparent px-3 font-sans text-sm text-text-primary no-underline transition-colors hover:border-accent hover:text-accent max-md:min-h-11'
</script>

<template>
  <div class="flex flex-col gap-10">
    <header class="band -mx-6 border-b border-line bg-band px-6 pt-8 pb-7 lg:-mx-10 lg:px-10 lg:pt-10">
      <RouterLink to="/containers" class="text-sm text-text-muted">Containers</RouterLink>
      <div class="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-2">
        <h1 class="m-0 text-3xl leading-tight font-semibold lg:text-4xl">
          {{ c?.name ?? repository.slice(repository.lastIndexOf('/') + 1) }}
          <span v-if="row" class="ml-2 font-mono text-2xl font-normal text-text-muted">{{ shownVersion(row) }}</span>
        </h1>
        <span
          v-if="row?.mark"
          class="rounded-full border border-current px-2.5 py-0.5 text-sm"
          :class="toneText[row.mark.tone]"
          >{{ row.mark.text }}</span
        >
        <GitHubLink v-if="source" :repository="source" class="ml-auto" />
      </div>
      <p v-if="loading" class="m-0 mt-4 text-text-muted">Loading…</p>
      <p v-else-if="unreadable" class="m-0 mt-4 text-text-muted">Not read: the cluster did not answer.</p>
      <p v-else-if="!c" class="m-0 mt-4 text-text-muted">The estate runs no image {{ repository }}.</p>
      <dl v-else class="m-0 mt-6 grid grid-cols-1 gap-x-10 gap-y-3 sm:grid-cols-2 lg:flex lg:flex-wrap">
        <div class="min-w-0">
          <dt :class="label">Image</dt>
          <dd class="m-0 font-mono text-sm break-all">{{ c.repository }}</dd>
        </div>
        <div>
          <dt :class="label">Version from</dt>
          <dd class="m-0 text-sm">{{ sourceWords(c, tagOf(c)) }}</dd>
        </div>
        <div>
          <dt :class="label">Digest</dt>
          <dd v-if="drifting" class="m-0 text-sm">
            <span class="font-mono">{{ short(drifting.runningDigest ?? '') }}</span>
            <span class="text-crit"> runs, but its pod pins {{ short(drifting.lockDigest ?? '') }}</span>
          </dd>
          <dd v-else-if="pinned" class="m-0 text-sm">
            <span class="font-mono">{{ short(pinned.lockDigest ?? '') }}</span>
            <span class="text-ok">matches the lock</span>
          </dd>
          <dd v-else class="m-0 text-sm text-text-muted">not pinned by digest</dd>
        </div>
        <div v-if="c.platform">
          <dt :class="label">Deployed by</dt>
          <dd class="m-0 text-sm">fleet-infra, until it moves to the estate path</dd>
        </div>
      </dl>
    </header>

    <div
      v-if="c"
      class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] lg:items-start lg:gap-12"
    >
      <div class="flex min-w-0 flex-col gap-10">
        <section v-if="spread(c)" aria-labelledby="spread-h">
          <h2 id="spread-h" class="m-0 mb-3 text-xl font-semibold">Versions running now</h2>
          <ul class="m-0 list-none border-t border-line p-0">
            <li
              v-for="(v, i) in versionsRunning(c)"
              :key="v.version"
              class="flex flex-wrap items-baseline gap-3 border-b border-line-soft py-2.5"
            >
              <b class="font-mono">{{ v.version }}</b>
              <span class="text-text-muted">{{ v.processes.join(', ') }}</span>
              <span v-if="i === 0" class="text-xs text-text-muted">newest</span>
            </li>
          </ul>
        </section>

        <section aria-labelledby="pods-h">
          <h2 id="pods-h" class="m-0 mb-3 text-xl font-semibold">Pods</h2>
          <table aria-label="Pods" class="w-full border-collapse">
            <thead class="hidden md:table-header-group">
              <tr class="border-b border-line">
                <th scope="col" :class="head">Project</th>
                <th scope="col" :class="head">Process</th>
                <th scope="col" :class="head">Node</th>
                <th scope="col" :class="head">Up</th>
                <th scope="col" :class="head"><span class="sr-only">Logs</span></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="i in c.instances"
                :key="i.pod + i.container"
                class="grid cursor-pointer grid-cols-[minmax(0,1fr)_auto] gap-x-3 border-b border-line-soft py-3 hover:bg-panel md:table-row md:py-0"
                :class="{ 'opacity-70': i.deleting }"
                @click="router.push(`/projects/${i.project}`)"
              >
                <th scope="row" class="text-left font-medium md:py-3 md:pr-4">
                  <RouterLink :to="`/projects/${i.project}`" @click.stop>{{ i.project }}</RouterLink>
                </th>
                <td class="col-start-1 text-sm md:table-cell md:py-3 md:pr-4 md:text-base">
                  {{ role(i) }}
                  <span v-if="spread(c) && i.version" class="ml-1 font-mono text-xs text-text-muted">{{
                    i.version
                  }}</span>
                  <span v-if="i.deleting" class="ml-1 text-xs text-back">stopping</span>
                </td>
                <td class="col-start-1 font-mono text-xs text-text-muted md:table-cell md:py-3 md:pr-4">
                  {{ i.node ?? '—' }}
                </td>
                <td class="col-start-1 text-sm text-text-muted md:table-cell md:py-3 md:pr-4">
                  <time v-if="i.started" :datetime="i.started">{{ age(i.started, now) }}</time>
                </td>
                <td class="col-start-2 row-span-4 row-start-1 md:table-cell md:py-3 md:text-right">
                  <button type="button" :class="button" @click.stop="logs = i">
                    Logs<span class="sr-only"> of {{ i.pod }}</span>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </section>

        <section aria-labelledby="history-h">
          <h2 id="history-h" class="m-0 mb-1 text-xl font-semibold">Version history</h2>
          <p class="m-0 mb-3 text-sm text-text-muted">
            Each version of the software this container ran. Releases that changed only our configuration are left out.
          </p>
          <p v-if="history.isPending.value" class="m-0 text-text-muted">Loading…</p>
          <p v-else-if="history.isError.value" class="m-0 text-text-muted">Not read: the cluster did not answer.</p>
          <p v-else-if="(history.data.value?.items ?? []).length === 0" class="m-0 text-text-muted">
            No version change recorded yet.
          </p>
          <ol v-else class="m-0 list-none border-t border-line p-0">
            <li
              v-for="(h, n) in history.data.value?.items ?? []"
              :key="h.at + h.version"
              class="flex flex-wrap items-baseline gap-x-4 gap-y-0.5 border-b border-line-soft py-2.5"
            >
              <span class="size-2 shrink-0 rounded-full" :class="n === 0 ? 'bg-accent' : 'bg-dim'" aria-hidden="true" />
              <b class="font-mono">{{ h.version }}</b>
              <span class="text-sm text-text-muted max-md:order-3 max-md:basis-full"
                >{{ h.from ? `from ${h.from}` : 'first recorded'
                }}<template v-if="h.release">
                  · shipped in
                  <RouterLink v-if="h.project" :to="`/projects/${h.project}`"
                    >{{ h.project }} {{ h.release }}</RouterLink
                  ></template
                ></span
              >
              <time class="ml-auto text-sm text-text-muted" :datetime="h.at">{{ age(h.at, now) }} ago</time>
            </li>
          </ol>
        </section>
      </div>

      <aside aria-labelledby="newer-h">
        <h2 id="newer-h" class="m-0 mb-3 text-xl font-semibold">Newer version</h2>
        <div class="border-t border-line pt-3">
          <p v-if="updatesUnreadable" class="m-0 text-text-muted">Not read: GitHub did not answer.</p>
          <template v-else-if="row?.newer">
            <p class="m-0 mb-3.5 text-text-muted">
              <b class="text-text-primary">{{ row.newer.version }} is ready to merge.</b>
              Renovate opened
              <a :href="row.newer.url">{{ row.newer.repository }}#{{ row.newer.number }}</a>
              {{ age(row.newer.opened, now) }} ago.
            </p>
            <a :href="row.newer.url" :class="[button, 'max-md:w-full max-md:justify-center']">Open the pull request</a>
          </template>
          <p v-else class="m-0 text-text-muted">
            {{
              c.platform
                ? 'Renovate watches this image in fleet-infra. Nothing newer is waiting.'
                : 'Nothing newer is waiting. Renovate opens a pull request when there is.'
            }}
          </p>
        </div>
      </aside>
    </div>

    <SideDrawer
      v-model:open="logsOpen"
      :title="logs?.container ?? ''"
      :subtitle="logs ? `pod ${logs.pod} · ${logs.version ?? ''}` : ''"
    >
      <ContainerLogs v-if="logs" :project="logs.project" :container="logs.container" :pod="logs.pod" />
    </SideDrawer>
  </div>
</template>
