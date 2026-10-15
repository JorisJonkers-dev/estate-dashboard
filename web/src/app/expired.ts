import type { Router } from 'vue-router'
import { zProblem } from '../infrastructure/api/zod.gen'

/** Sends the browser to sign in when a read gets a 401 mid-visit. */
export function backToSignIn(router: Router): (error: unknown) => void {
  return (error) => {
    const problem = zProblem.safeParse(error)
    if (!problem.success || problem.data.status !== 401) return
    const here = router.currentRoute.value
    if (here.meta.public) return
    void router.replace({ name: 'sign-in', query: here.fullPath === '/' ? {} : { next: here.fullPath } })
  }
}
