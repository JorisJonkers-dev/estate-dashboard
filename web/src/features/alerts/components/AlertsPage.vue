<script setup lang="ts">
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, ref } from 'vue'
import { listAlertHistoryOptions, listAlertsOptions, listAlertsQueryKey, listIssuesOptions, listReleasesOptions, silenceAlertMutation } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { useNow } from '../../../infrastructure/useNow'
import { toneText } from '../../../shared/tone'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import { age } from '../../overview/domain/overview'
import { describeError } from '../composables/useAlertHistory'
import { describeEvent } from '../domain/eventText'
import { classes, inbox, type AlertClass } from '../domain/inbox'

const live = { refetchInterval: 30_000 }
const alerts = useQuery({ ...listAlertsOptions(), ...live })
const issues = useQuery({ ...listIssuesOptions(), ...live })
const releases = useQuery({ ...listReleasesOptions(), ...live })
const history = useQuery({ ...listAlertHistoryOptions(), ...live })
const now = useNow()

const items = computed(() => inbox(alerts.data.value?.items ?? [], issues.data.value?.items ?? [], releases.data.value?.items ?? []))
const alertmanager = computed(() => (alerts.error.value ? describeError(alerts.error.value) : null))

const tab = ref<'All' | AlertClass>('All')
const shown = computed(() => items.value.filter((i) => tab.value === 'All' || i.class === tab.value))
const count = (c: 'All' | AlertClass) => items.value.filter((i) => c === 'All' || i.class === c).length

const pickedKey = ref<string | null>(null)
const picked = computed(() => shown.value.find((i) => i.key === pickedKey.value) ?? shown.value[0])
const pickedHistory = computed(() => (history.data.value?.items ?? []).filter((e) => picked.value?.fingerprint !== undefined && e.fingerprint === picked.value.fingerprint))

// The one write here. It goes to Alertmanager, which also silences Discord.
const queryClient = useQueryClient()
const length = ref<'1h' | '4h' | '1d' | 'until-resolved'>('1h')
const silence = useMutation(silenceAlertMutation())
function silencePicked() {
  if (!picked.value?.fingerprint) return
  silence.mutate(
    { path: { fingerprint: picked.value.fingerprint }, body: { length: length.value } },
    { onSuccess: () => void queryClient.invalidateQueries({ queryKey: listAlertsQueryKey() }) },
  )
}

const label = 'text-xs font-semibold tracking-[0.12em] text-text-muted uppercase'
const when = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand eyebrow="Production" title="Alerts">From Alertmanager, open estate issues and the Release Gate. A silence here also silences Discord.</HeadBand>

    <p v-if="alertmanager" role="status" class="-mt-4 mb-0 text-[15px] text-warn">{{ alertmanager }}</p>

    <div role="tablist" aria-label="Alert class" class="flex gap-6 overflow-x-auto border-b border-line">
      <button
        v-for="c in ['All', ...classes] as const"
        :key="c"
        role="tab"
        type="button"
        :aria-selected="tab === c"
        class="-mb-px min-h-11 shrink-0 cursor-pointer border-0 border-b-2 border-transparent bg-transparent px-0.5 font-sans text-[15px] whitespace-nowrap text-nav transition-colors hover:text-white aria-selected:border-accent aria-selected:text-white"
        @click="tab = c"
      >
        {{ c }} <span class="text-dim">{{ count(c) }}</span>
      </button>
    </div>

    <EmptyPlate v-if="shown.length === 0" title="All clear">Nothing here needs you.</EmptyPlate>
    <div v-else class="flex flex-col gap-8 lg:grid lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] lg:items-start lg:gap-12">
      <ul aria-label="Alerts" class="m-0 list-none border-t border-line p-0">
        <li v-for="i in shown" :key="i.key">
          <button
            type="button"
            :aria-pressed="picked?.key === i.key"
            class="grid w-full cursor-pointer grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 border-0 border-b border-l-2 border-line-soft border-l-transparent bg-transparent px-3 py-3 text-left font-sans text-text-primary transition-colors hover:bg-panel aria-pressed:border-l-accent aria-pressed:bg-panel"
            @click="pickedKey = i.key"
          >
            <span class="size-2 rounded-full" :class="i.silenced ? 'bg-dim' : ['bg-current', toneText[i.tone]]" aria-hidden="true" />
            <span class="min-w-0">
              <span class="block truncate font-medium">{{ i.title }}</span>
              <span class="block text-sm text-text-muted">{{ i.severity }} · {{ i.class }}<template v-if="i.project"> · {{ i.project }}</template></span>
            </span>
            <span class="text-right text-sm text-text-muted">
              <span class="block">{{ age(i.since, now) }}</span>
              <span v-if="i.silenced" class="block">silenced</span>
            </span>
          </button>
        </li>
      </ul>

      <section v-if="picked" aria-labelledby="detail-h" class="flex flex-col gap-4 lg:border-l lg:border-line lg:pl-8">
        <p class="m-0 text-xs font-semibold tracking-[0.12em] uppercase" :class="toneText[picked.tone]">{{ picked.severity }} · {{ picked.class }}</p>
        <h2 id="detail-h" class="m-0 text-2xl font-semibold">{{ picked.title }}</h2>
        <dl class="m-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-5 gap-y-1.5 text-[15px]">
          <template v-if="picked.project">
            <dt class="text-text-muted">Project</dt>
            <dd class="m-0"><RouterLink :to="`/projects/${picked.project}`">{{ picked.project }}</RouterLink></dd>
          </template>
          <dt class="text-text-muted">Source</dt>
          <dd class="m-0">{{ picked.source }}</dd>
          <dt class="text-text-muted">First seen</dt>
          <dd class="m-0">{{ when.format(new Date(picked.since)) }}</dd>
          <template v-if="picked.labels.length">
            <dt class="text-text-muted">Evidence</dt>
            <dd class="m-0 font-mono text-[13px] break-all text-text-muted">
              <span v-for="[k, v] in picked.labels" :key="k" class="block">{{ k }}={{ v }}</span>
            </dd>
          </template>
        </dl>
        <div class="flex flex-wrap items-center gap-2">
          <template v-if="picked.fingerprint && !picked.silenced">
            <label class="flex items-center gap-2 text-[15px] text-text-muted">
              Silence for
              <select v-model="length" class="min-h-10 rounded-md border border-control bg-surface-dark px-2 font-sans text-text-primary">
                <option value="1h">1 hour</option>
                <option value="4h">4 hours</option>
                <option value="1d">1 day</option>
                <option value="until-resolved">until resolved</option>
              </select>
            </label>
            <button type="button" :disabled="silence.isPending.value" class="min-h-10 cursor-pointer rounded-md border border-accent bg-accent px-4 font-sans text-[15px] font-medium text-[#1c1c1c] transition-colors hover:bg-transparent hover:text-accent disabled:opacity-50" @click="silencePicked">
              Silence
            </button>
          </template>
          <RouterLink v-if="picked.link && picked.link.href.startsWith('/')" :to="picked.link.href" class="inline-flex min-h-10 items-center rounded-md border border-control px-4 text-[15px] font-medium no-underline hover:border-accent">{{ picked.link.label }}</RouterLink>
          <a v-else-if="picked.link" :href="picked.link.href" target="_blank" rel="noopener" class="inline-flex min-h-10 items-center rounded-md border border-control px-4 text-[15px] font-medium no-underline hover:border-accent">{{ picked.link.label }}<span class="sr-only"> (opens in a new tab)</span></a>
        </div>
        <p v-if="silence.isError.value" role="alert" class="m-0 text-crit">{{ describeError(silence.error.value) }}</p>
        <div v-if="picked.fingerprint" class="border-t border-line pt-4">
          <h3 class="m-0 mb-2" :class="label">History</h3>
          <p v-if="pickedHistory.length === 0" class="m-0 text-sm text-text-muted">Nothing recorded yet.</p>
          <ol v-else class="m-0 flex list-none flex-col gap-1 p-0 text-[15px]">
            <li v-for="e in pickedHistory" :key="e.id" class="flex gap-4">
              <time class="min-w-36 text-text-muted" :datetime="e.observedAt">{{ when.format(new Date(e.observedAt)) }}</time>
              <span>{{ describeEvent(e) }}</span>
            </li>
          </ol>
        </div>
      </section>
    </div>
  </div>
</template>
