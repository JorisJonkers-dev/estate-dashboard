<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import ConfirmAction from '../../../shared/ui/ConfirmAction.vue'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import GitHubLink from '../../../shared/ui/GitHubLink.vue'
import SideDrawer from '../../../shared/ui/SideDrawer.vue'
import StatusStrip from '../../../shared/ui/StatusStrip.vue'
import { age } from '../../overview/domain/overview'
import { useProject } from '../composables/useProject'
import { healthBlocks } from '../domain/delivery'
import { heldBackActions, heldBackText, links } from '../domain/project'
import ChecksPanel from './ChecksPanel.vue'
import ContainerLogs from './ContainerLogs.vue'
import DeliveryPanel from './DeliveryPanel.vue'
import NotesView from './NotesView.vue'
import ReleasesList from './ReleasesList.vue'
import RunningNow from './RunningNow.vue'

const route = useRoute()
const name = computed(() => String(route.params.name))
const {
  isLoading,
  row,
  repository,
  pin,
  alerts,
  releases,
  heldBack,
  streams,
  rolloutsLoading,
  pods,
  podsLoading,
  podsUnreadable,
  checks,
  checksUnreadable,
  checksLoading,
  delivery,
  health,
  healthUnreadable,
  updates,
} = useProject(name)
const now = useNow()

const legacy = computed(() => row.value?.path === 'legacy')
/** The release pinned last: its pods are what runs now, any other's are still on an older release. */
const pinned = computed(() => releases.value[0]?.version ?? row.value?.version)
const strip = computed(() => (health.value ? healthBlocks(health.value, now.value) : []))
/** Each container's version in the release running now. */
const versions = computed(
  () =>
    new Map(
      pods.value
        .filter((p) => p.version === pinned.value)
        .flatMap((p) =>
          p.containers.filter((c) => c.version !== undefined).map((c) => [c.name, c.version ?? ''] as const),
        ),
    ),
)

// One drawer, for a container's logs or a release's changelog.
type Drawer =
  { kind: 'logs'; container: string; subtitle: string } | { kind: 'notes'; version: string; subtitle: string }
const drawer = ref<Drawer | null>(null)
const drawerOpen = computed({
  get: () => drawer.value !== null,
  set: (v: boolean) => {
    if (!v) drawer.value = null
  },
})
function showLogs(container: string, version: string | undefined) {
  drawer.value = {
    kind: 'logs',
    container,
    subtitle: [name.value, pinned.value ? `release ${pinned.value}` : '', version ? `${container} ${version}` : '']
      .filter(Boolean)
      .join(' · '),
  }
}
function showNotes(version: string) {
  drawer.value = { kind: 'notes', version, subtitle: 'Changelog' }
}
const drawerTitle = computed(() =>
  drawer.value?.kind === 'logs' ? drawer.value.container : drawer.value ? `${name.value} ${drawer.value.version}` : '',
)

const primary =
  'inline-flex min-h-10 grow items-center justify-center rounded-md border border-accent bg-accent px-4 font-sans text-[15px] font-medium text-black no-underline transition-colors hover:bg-accent/85 hover:text-black max-md:min-h-11'
</script>

<template>
  <div class="flex flex-col gap-10">
    <header class="-mx-6 border-b border-line bg-band px-6 pt-8 pb-7 lg:-mx-10 lg:px-10 lg:pt-10 lg:pb-8">
      <div class="flex flex-wrap items-end justify-between gap-6">
        <div class="min-w-0">
          <p class="m-0 text-xs font-semibold tracking-[0.2em] text-text-muted uppercase">
            {{ legacy ? 'Old path, deployed by fleet-infra' : 'Estate path' }}
          </p>
          <h1 class="m-0 mt-1.5 text-3xl leading-tight font-semibold lg:text-4xl">
            {{ name
            }}<span v-if="row?.version" class="ml-3 font-mono text-2xl font-normal text-text-muted">{{
              row.version
            }}</span>
          </h1>
          <p v-if="!isLoading && !row" class="m-0 mt-2 text-text-muted">This project isn't in the estate.</p>
          <p v-else-if="pin?.paused" class="m-0 mt-2 text-back">
            Paused by {{ pin.paused.by }}<template v-if="pin.paused.reason">: {{ pin.paused.reason }}</template>
          </p>
          <div v-else-if="heldBack" role="status" class="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1">
            <p class="m-0 text-warn">{{ heldBackText(heldBack) }}.</p>
            <a v-for="a in heldBackActions(heldBack)" :key="a.label" :href="a.href" class="text-[15px]">{{
              a.label
            }}</a>
          </div>
        </div>
        <div v-if="row && !legacy" class="flex w-full flex-col gap-2.5 md:w-[28rem]">
          <StatusStrip
            v-if="health"
            :blocks="strip"
            :label="`Health of ${name} over the last hour, one block per ${String(health.step)} seconds`"
          />
          <p v-else-if="healthUnreadable" class="m-0 text-right text-xs text-text-muted">
            Health over the last hour: not read
          </p>
          <div class="grid grid-cols-3 gap-2 md:flex">
            <RouterLink :to="`/projects/${name}/rollout`" :class="[primary, 'col-span-3']">Latest rollout</RouterLink>
            <GitHubLink v-if="repository" :repository="repository" class="w-full md:w-10" />
            <template v-if="pin">
              <ConfirmAction
                v-if="pin.paused"
                label="Resume"
                :title="`Resume ${name}?`"
                :text="`New releases of ${name} roll out again. This opens the resume workflow on GitHub.`"
                action="Open the resume workflow"
                :href="links.workflow('resume')"
              />
              <ConfirmAction
                v-else
                label="Pause"
                :title="`Pause ${name}?`"
                :text="`New releases of ${name} are held back. What runs now keeps running. This opens the pause workflow on GitHub, which records who paused it and why.`"
                action="Open the pause workflow"
                :href="links.workflow('pause')"
              />
              <ConfirmAction
                label="Roll back"
                :title="`Roll back ${name}?`"
                :text="`${name} goes back to the last release that works with the current database, after a backup. It stays paused until you resume it. This opens the rollback workflow on GitHub.`"
                action="Open the rollback workflow"
                :href="links.workflow('rollback')"
              />
            </template>
          </div>
        </div>
      </div>
    </header>

    <EmptyPlate v-if="legacy" title="Still on the old path" mark="quiet">
      fleet-infra deploys {{ name }} for now. History shows up here once it moves to the estate path.
    </EmptyPlate>

    <div
      v-else
      class="flex flex-col gap-10 lg:grid lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] lg:items-start lg:gap-12"
    >
      <div class="flex min-w-0 flex-col gap-10">
        <RunningNow
          :project="name"
          :release="pinned"
          :pods="pods"
          :updates="updates"
          :loading="podsLoading"
          :unreadable="podsUnreadable"
          :now="now"
          @logs="showLogs"
        />
        <ReleasesList
          :key="name"
          :project="name"
          :releases="releases"
          :streams="streams"
          :versions="versions"
          :current="pinned"
          :changelog="repository !== undefined"
          :loading="rolloutsLoading"
          :now="now"
          @notes="showNotes"
        />
      </div>

      <div class="flex min-w-0 flex-col gap-10">
        <section aria-labelledby="alerts-h">
          <div class="mb-3 flex items-end justify-between gap-4">
            <h2 id="alerts-h" class="m-0 text-xl font-semibold">Alerts</h2>
            <RouterLink to="/alerts" class="text-[15px]">All alerts</RouterLink>
          </div>
          <p v-if="alerts.length === 0" class="text-text-muted">All clear.</p>
          <ul v-else class="m-0 list-none border-t border-line p-0">
            <li v-for="a in alerts" :key="a.key" class="flex flex-col gap-0.5 border-b border-line-soft py-3">
              <span class="flex items-center gap-2 text-[13px]">
                <span
                  class="size-2 rounded-full"
                  :class="a.silenced ? 'bg-dim' : ['bg-current', toneText[a.tone]]"
                  aria-hidden="true"
                />
                <span class="font-semibold" :class="a.silenced ? 'text-dim' : toneText[a.tone]">{{ a.severity }}</span>
                <span class="text-text-muted">{{ a.class }}<template v-if="a.silenced"> · silenced</template></span>
                <time class="ml-auto text-text-muted" :datetime="a.since">{{ age(a.since, now) }}</time>
              </span>
              <RouterLink
                :to="{ path: '/alerts', query: { alert: a.key } }"
                class="font-medium no-underline hover:underline"
                >{{ a.title }}</RouterLink
              >
            </li>
          </ul>
        </section>
        <ChecksPanel
          :project="name"
          :checks="checks"
          :loading="checksLoading"
          :unreadable="checksUnreadable"
          :now="now"
        />
        <DeliveryPanel :project="name" :facts="delivery" />
      </div>
    </div>

    <SideDrawer v-model:open="drawerOpen" :title="drawerTitle" :subtitle="drawer?.subtitle ?? ''">
      <ContainerLogs
        v-if="drawer?.kind === 'logs'"
        :key="drawer.container"
        :project="name"
        :container="drawer.container"
      />
      <NotesView
        v-else-if="drawer?.kind === 'notes' && repository"
        :repository="repository"
        :version="drawer.version"
      />
    </SideDrawer>
  </div>
</template>
