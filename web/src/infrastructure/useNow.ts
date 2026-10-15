import { onScopeDispose, ref, type Ref } from 'vue'

/** The time, moved on every `every` milliseconds, so an age on the page grows while it is open. */
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
