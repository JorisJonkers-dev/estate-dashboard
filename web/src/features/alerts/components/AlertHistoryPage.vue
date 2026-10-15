<script setup lang="ts">
import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import { listAlertsOptions } from '../../../infrastructure/api/@tanstack/vue-query.gen'
import EmptyPlate from '../../../shared/ui/EmptyPlate.vue'
import HeadBand from '../../../shared/ui/HeadBand.vue'
import { describeError, useAlertHistory } from '../composables/useAlertHistory'
import { describeEvent } from '../domain/eventText'

const { events, isLoading, loadError } = useAlertHistory()
// Without Alertmanager nothing fires or gets recorded.
const live = useQuery({ ...listAlertsOptions(), refetchInterval: 30_000 })
const alertmanager = computed(() => (live.error.value ? describeError(live.error.value) : null))

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <div class="flex flex-col gap-10">
    <HeadBand eyebrow="Alerts" title="Alert history">Every alert that fired, and when it cleared.</HeadBand>

    <p v-if="alertmanager" role="status" class="-mt-4 mb-0 text-[15px] text-warn">{{ alertmanager }}</p>

    <p v-if="isLoading" class="m-0 text-text-muted">Loading…</p>
    <p v-else-if="loadError" role="alert" class="m-0 text-crit">{{ loadError }}</p>
    <EmptyPlate v-else-if="events.length === 0" title="No alerts yet" :mark="alertmanager ? 'quiet' : 'tick'">
      Nothing has fired since the dashboard started.
    </EmptyPlate>
    <ul v-else aria-label="Alert history" class="m-0 list-none border-t border-line p-0">
      <li
        v-for="(event, i) in events"
        :key="event.id"
        class="rise grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 border-b border-line-soft py-3 md:grid-cols-[auto_minmax(0,1fr)_auto_auto]"
        :style="{ '--i': Math.min(i, 12) }"
      >
        <span class="size-2 rounded-full" :class="event.status === 'firing' ? 'bg-warn' : 'bg-ok'" aria-hidden="true" />
        <span class="name font-medium">{{ event.name }}</span>
        <span
          class="state whitespace-nowrap"
          :class="event.status === 'firing' ? 'text-warn' : 'text-ok'"
          :data-status="event.status"
          >{{ describeEvent(event) }}</span
        >
        <time
          class="col-span-3 pl-5 text-sm whitespace-nowrap text-text-muted md:col-span-1 md:pl-0"
          :datetime="event.observedAt"
          >{{ dateFormat.format(new Date(event.observedAt)) }}</time
        >
      </li>
    </ul>
  </div>
</template>
