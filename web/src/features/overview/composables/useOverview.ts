import { useQueries, useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  getHandoverOptions,
  listAlertsOptions,
  listDeploysOptions,
  listIssuesOptions,
  listPinsOptions,
  listReleasesOptions,
  listUnitsOptions,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import type { Deploy } from '../../../infrastructure/api/types.gen'
import { describeError } from '../../alerts/composables/useAlertHistory'
import { health, needs, projectRows, type Reads } from '../domain/overview'

/** How often the Overview reads again. */
const every = 30_000

/** Server state for the Overview. A source that fails is named in `unread`. */
export function useOverview() {
  const live = { refetchInterval: every }
  const handover = useQuery({ ...getHandoverOptions(), ...live })
  const pins = useQuery({ ...listPinsOptions(), ...live })
  const units = useQuery({ ...listUnitsOptions(), ...live })
  const releases = useQuery({ ...listReleasesOptions(), ...live })
  const issues = useQuery({ ...listIssuesOptions(), ...live })
  const alerts = useQuery({ ...listAlertsOptions(), ...live })

  // The newest deploy of each pinned Project.
  const pinned = computed(() => (pins.data.value?.items ?? []).map((p) => p.project).filter((p) => !p.startsWith('_')))
  const deploys = useQueries({
    queries: computed(() =>
      pinned.value.map((project) => ({ ...listDeploysOptions({ path: { project }, query: { limit: 1 } }), ...live })),
    ),
  })

  const reads = computed<Reads>(() => {
    const newest: Record<string, Deploy | undefined> = {}
    pinned.value.forEach((project, i) => {
      newest[project] = deploys.value[i]?.data?.items[0]
    })
    const out: Reads = { deploys: newest }
    if (handover.data.value) out.handover = handover.data.value
    if (pins.data.value) out.pins = pins.data.value.items
    if (units.data.value) out.units = units.data.value.items
    if (releases.data.value) out.releases = releases.data.value.items
    if (issues.data.value) out.issues = issues.data.value.items
    if (alerts.data.value) out.alerts = alerts.data.value.items
    return out
  })

  const sources = [
    ['the estate repository', handover],
    ['the cluster', units],
    ['the Release Gate', releases],
    ['Alertmanager', alerts],
  ] as const

  return {
    isLoading: computed(() => handover.isPending.value || units.isPending.value),
    projects: computed(() => projectRows(reads.value)),
    health: computed(() => health(reads.value)),
    needs: computed(() => needs(reads.value)),
    /** Firing alerts not silenced. The nav shows this count. */
    firing: computed(() => (alerts.data.value?.items ?? []).filter((a) => a.silencedBy.length === 0).length),
    /** What could not be read, and why. */
    unread: computed(() =>
      sources.flatMap(([name, query]) => (query.error.value ? [{ name, why: describeError(query.error.value) }] : [])),
    ),
  }
}
