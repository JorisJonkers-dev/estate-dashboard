import { useQuery } from '@tanstack/vue-query'
import { computed } from 'vue'
import {
  listContainersOptions,
  listUnitsOptions,
  listUpdatesOptions,
} from '../../../infrastructure/api/@tanstack/vue-query.gen'
import { rowOf } from '../domain/containers'

const live = { refetchInterval: 30_000 }

/** Every image the estate runs, as rows, with the reads behind them. GitHub not answering blanks only what it says. */
export function useContainers() {
  const containers = useQuery({ ...listContainersOptions(), ...live })
  const updates = useQuery({ ...listUpdatesOptions(), refetchInterval: 300_000 })
  const units = useQuery({ ...listUnitsOptions(), ...live })
  return {
    loading: computed(() => containers.isPending.value),
    unreadable: computed(() => containers.isError.value),
    updatesUnreadable: computed(() => updates.isError.value),
    rows: computed(() => (containers.data.value?.items ?? []).map((c) => rowOf(c, updates.data.value?.items ?? []))),
    /** The repository a Project publishes from, from its unit. */
    repositoryOf: (project: string) =>
      (units.data.value?.items ?? []).find((u) => u.source === `project-${project}`)?.repository,
  }
}
