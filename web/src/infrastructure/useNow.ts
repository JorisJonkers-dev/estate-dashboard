import { onScopeDispose, ref, type Ref } from 'vue'

/** The time, updated every `every` milliseconds, so ages keep growing. */
export function useNow(every = 30_000): Ref<number> {
  const now = ref(Date.now())
  const tick = setInterval(() => {
    now.value = Date.now()
  }, every)
  onScopeDispose(() => {
    clearInterval(tick)
  })
  return now
}
