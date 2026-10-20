import { useQuery } from '@tanstack/vue-query'
import { computed, type Ref } from 'vue'
import {
  getHandoverOptions,
  getHealthOptions,
  listAlertsOptions,
  listChecksOptions,
  listDeploysOptions,
  listIssuesOptions,
  listLogStreamsOptions,
  listPinsOptions,
  listPodsOptions,
  listReleasesOptions,
  listRolloutsOptions,
  listSourcesOptions,
  listUnitsOptions,
  listUpdatesOptions,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { inbox } from '../../alerts/domain/inbox'
import { projectRows } from '../../overview/domain/overview'
import { window } from '../domain/logs'
import { checksOf } from '../domain/health'
import { deliveryFacts } from '../domain/delivery'
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
  // Live state reads more often: a container that stops answering should show within seconds.
  const pods = useQuery(
    computed(() => ({ ...listPodsOptions({ path: { project: name.value } }), refetchInterval: 10_000 })),
  )
  const checks = useQuery({ ...listChecksOptions(), refetchInterval: 30_000 })
  const updates = useQuery({ ...listUpdatesOptions(), refetchInterval: 300_000 })
  const probing = computed(() =>
    checksOf(
      name.value,
      [...new Set((pods.data.value?.items ?? []).map((p) => p.process).filter(Boolean))],
      checks.data.value?.items ?? [],
    ),
  )
  // The last hour, once the checks that probe it are known.
  const health = useQuery(
    computed(() => ({
      ...getHealthOptions({ path: { project: name.value }, query: { check: probing.value.map((c) => c.key) } }),
      enabled: !checks.isPending.value,
      refetchInterval: 30_000,
      retry: false,
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
    delivery: computed(() =>
      deliveryFacts(
        name.value,
        sources.data.value?.items ?? [],
        units.data.value?.items ?? [],
        releases.data.value?.items ?? [],
      ),
    ),
    health: computed(() => health.data.value),
    gate: computed(() => (releases.data.value?.items ?? []).find((r) => r.namespace === `${name.value}-system`)),
    healthUnreadable: computed(() => health.isError.value),
    updates: computed(() => updates.data.value?.items ?? []),
    alerts: computed(() =>
      inbox(alerts.data.value?.items ?? [], issues.data.value?.items ?? [], releases.data.value?.items ?? []).filter(
        (i) => i.project === name.value,
      ),
    ),
    releases: computed(() => releaseViews(rollouts.data.value?.items ?? [], unitOf.value)),
    streams: computed(() => streams.data.value?.items ?? []),
    pods: computed(() => pods.data.value?.items ?? []),
    podsLoading: computed(() => pods.isPending.value),
    podsUnreadable: computed(() => pods.isError.value),
    checks: probing,
    checksUnreadable: computed(() => checks.isError.value),
    checksLoading: computed(() => checks.isPending.value),
    rolloutsLoading: computed(() => repository.value !== undefined && rollouts.isPending.value),
  }
}
