import { useQuery } from '@tanstack/vue-query'
import { computed, type Ref } from 'vue'
import {
  getHandoverOptions,
  listAlertsOptions,
  listDeploysOptions,
  listIssuesOptions,
  listLogStreamsOptions,
  listPinsOptions,
  listReleasesOptions,
  listRolloutsOptions,
  listSourcesOptions,
  listUnitsOptions,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { inbox } from '../../alerts/domain/inbox'
import { projectRows } from '../../overview/domain/overview'
import { window } from '../domain/logs'
import { clusterRows } from '../domain/project'
import { releases as releaseViews } from '../domain/releases'

const live = { refetchInterval: 30_000 }

/** What the dashboard knows about one Project. */
export function useProject(name: Ref<string>) {
  const handover = useQuery({ ...getHandoverOptions(), ...live })
  const pins = useQuery({ ...listPinsOptions(), ...live })
  const units = useQuery({ ...listUnitsOptions(), ...live })
  const sources = useQuery({ ...listSourcesOptions(), ...live })
  const releases = useQuery({ ...listReleasesOptions(), ...live })
  const issues = useQuery({ ...listIssuesOptions(), ...live })
  const alerts = useQuery({ ...listAlertsOptions(), ...live })
  const deploys = useQuery(
    computed(() => ({ ...listDeploysOptions({ path: { project: name.value }, query: { limit: 20 } }), ...live })),
  )

  /** The repository the Project publishes from. */
  const repository = computed(
    () => (units.data.value?.items ?? []).find((u) => u.source === `project-${name.value}` && u.repository)?.repository,
  )
  const rollouts = useQuery(
    computed(() => ({
      ...listRolloutsOptions({
        path: { project: name.value },
        query: { repository: repository.value ?? '', limit: 10 },
      }),
      enabled: repository.value !== undefined,
      ...live,
    })),
  )

  const streams = useQuery(
    computed(() => ({
      ...listLogStreamsOptions({ path: { project: name.value }, query: { hours: window } }),
      refetchInterval: 60_000,
    })),
  )
  const unitOf = computed(() => (units.data.value?.items ?? []).find((u) => u.source === `project-${name.value}`))

  const row = computed(() =>
    projectRows({
      ...(handover.data.value ? { handover: handover.data.value } : {}),
      ...(pins.data.value ? { pins: pins.data.value.items } : {}),
      ...(units.data.value ? { units: units.data.value.items } : {}),
      ...(releases.data.value ? { releases: releases.data.value.items } : {}),
      ...(issues.data.value ? { issues: issues.data.value.items } : {}),
      deploys: { [name.value]: deploys.data.value?.items[0] },
    }).find((r) => r.name === name.value),
  )

  return {
    isLoading: computed(() => handover.isPending.value || pins.isPending.value),
    row,
    repository,
    pin: computed(() => pins.data.value?.items.find((p) => p.project === name.value)),
    unit: unitOf,
    cluster: computed(() =>
      clusterRows(
        name.value,
        sources.data.value?.items ?? [],
        units.data.value?.items ?? [],
        releases.data.value?.items ?? [],
      ),
    ),
    alerts: computed(() =>
      inbox(alerts.data.value?.items ?? [], issues.data.value?.items ?? [], releases.data.value?.items ?? []).filter(
        (i) => i.project === name.value,
      ),
    ),
    deploys: computed(() => deploys.data.value?.items ?? []),
    releases: computed(() => releaseViews(rollouts.data.value?.items ?? [], unitOf.value)),
    streams: computed(() => streams.data.value?.items ?? []),
    rolloutsLoading: computed(() => repository.value !== undefined && rollouts.isPending.value),
  }
}
