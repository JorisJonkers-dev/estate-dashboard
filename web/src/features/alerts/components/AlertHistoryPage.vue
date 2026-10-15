<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { listAlertsOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import { describeError, useAlertHistory } from '../composables/useAlertHistory'
import { describeEvent } from '../domain/eventText'

const { events, isLoading, loadError } = useAlertHistory()
// Whether Alertmanager answers at all: without it nothing fires and nothing is recorded.
const live = useQuery({ ...listAlertsOptions(), refetchInterval: 30_000 })
const alertmanager = computed(() => (live.error.value ? describeError(live.error.value) : null))

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <div class="flex flex-col gap-8 md:gap-10">
    <HeadBand eyebrow="Alerts" title="Alert history" accent="var(--color-warn)">
      Every alert that fired, and when it cleared.
    </HeadBand>

    <p v-if="alertmanager" role="status" class="m-0 text-[15px] text-warn">{{ alertmanager }}</p>

    <p v-if="isLoading" class="m-0 text-text-muted">Loading the history…</p>
    <p v-else-if="loadError" role="alert" class="m-0 text-crit">{{ loadError }}</p>
    <EmptyPlate v-else-if="events.length === 0" title="No alerts yet" :mark="alertmanager ? 'quiet' : 'tick'">
      Nothing has fired since the dashboard started keeping track.
    </EmptyPlate>
    <ul v-else aria-label="Alert history" class="m-0 flex list-none flex-col gap-1.5 p-0">
      <li
        v-for="(event, i) in events"
        :key="event.id"
        class="event rise relative grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-0.5 py-3 pr-4 pl-7 md:grid-cols-[minmax(0,1fr)_auto_auto]"
        :class="event.status === 'firing' ? 'text-warn' : 'text-ok'"
        :style="{ '--i': Math.min(i, 12) }"
      >
        <span class="name font-semibold text-text-primary">{{ event.name }}</span>
        <span class="state font-semibold whitespace-nowrap" :data-status="event.status">{{ describeEvent(event) }}</span>
        <time class="col-span-2 text-sm whitespace-nowrap text-text-muted md:col-span-1 md:text-base" :datetime="event.observedAt">{{ dateFormat.format(new Date(event.observedAt)) }}</time>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.event {
  background: linear-gradient(90deg, color-mix(in oklab, currentColor 9%, transparent), transparent 60%), var(--color-band);
}
.event::before {
  content: '';
  position: absolute;
  top: 0.7rem;
  bottom: 0.7rem;
  left: 0.75rem;
  width: 3px;
  background: currentColor;
  transform: skewX(-12deg);
}
</style>
