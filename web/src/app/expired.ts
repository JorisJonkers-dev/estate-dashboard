import type { Router } from 'vue-router'
import { zProblem } from '../infrastructure/api/zod.gen'

/**
 * Sends the browser back to sign in when a read is refused with a 401 mid-visit: the session
 * ended while the page was open. The router asks who is signed in only on navigation, so this is
 * what catches a session that lapses between two.
 */
export function backToSignIn(router: Router): (error: unknown) => void {
  return (error) => {
    const problem = zProblem.safeParse(error)
    if (!problem.success || problem.data.status !== 401) return
    const here = router.currentRoute.value
    if (here.meta.public) return
    void router.replace({ name: 'sign-in', query: here.fullPath === '/' ? {} : { next: here.fullPath } })
  }
}
