<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { listLogStreamsOptions, listReleasesOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { window } from '../domain/logs'
import LogView from './LogView.vue'

/** What one container wrote, in one pod or each pod that runs it, newest pod first. */
const props = defineProps<{ project: string; container: string; pod?: string | undefined }>()
const streams = useQuery(
  computed(() => ({ ...listLogStreamsOptions({ path: { project: props.project }, query: { hours: window } }) })),
)
const of = computed(() =>
  (streams.data.value?.items ?? [])
    .filter((s) => s.container === props.container && (props.pod === undefined || s.pod === props.pod))
    .sort((a, b) => (b.first ?? '').localeCompare(a.first ?? '')),
)
// A Process the Release Gate lists as a member is switched by Flagger, so its pods are named by role
// even with no primary listed.
const releases = useQuery(listReleasesOptions())
const gated = computed(() =>
  (releases.data.value?.items ?? []).some(
    (r) => r.namespace === `${props.project}-system` && r.members.some((m) => m.process === props.container),
  ),
)
</script>

<template>
  <p v-if="streams.isPending.value" class="m-0 text-text-muted">Loading…</p>
  <p v-else-if="streams.isError.value" class="m-0 text-text-muted">Loki did not answer.</p>
  <p v-else-if="of.length === 0" class="m-0 text-text-muted">{{ container }} wrote nothing in the last hours.</p>
  <LogView v-else :key="of[0]?.pod" :project="project" :streams="of" :gated="gated" />
</template>
