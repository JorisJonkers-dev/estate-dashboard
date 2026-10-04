import { createRouter, createWebHistory, type Router, type RouterHistory } from 'vue-router'
import type { SignedIn } from '../features/session/composables/useSession'

declare module 'vue-router' {
  interface RouteMeta {
    /** A page a browser reaches without a session: it draws its own frame, and nothing asks who is there. */
    public?: boolean
  }
}

/**
 * Asks the API who is signed in. The generated client loads on demand, like a page: it builds its
 * validators as it loads, and that must wait for main.ts to configure zod for the server's
 * Content-Security-Policy.
 */
async function askTheApi(): Promise<SignedIn> {
  const { whoIsSignedIn } = await import('../features/session/composables/useSession')
  return whoIsSignedIn()
}

/**
 * The app's routes. Each feature page loads on demand, so the first paint carries only the shell.
 * Every page but the two public ones needs a signed-in admin: the API refuses anyone else, and a
 * browser the API refuses is sent to sign in, to come back to the page it asked for.
 */
export function createAppRouter(history: RouterHistory = createWebHistory(), signedIn: () => Promise<SignedIn> = askTheApi): Router {
  const router = createRouter({
    history,
    routes: [
      { path: '/', name: 'alert-history', component: () => import('../features/alerts/components/AlertHistoryPage.vue') },
      { path: '/sign-in', name: 'sign-in', meta: { public: true }, component: () => import('../features/session/components/SignInPage.vue') },
      {
        path: '/not-an-admin',
        name: 'not-an-admin',
        meta: { public: true },
        component: () => import('../features/session/components/NotAnAdminPage.vue'),
      },
      { path: '/:rest(.*)*', redirect: '/' },
    ],
  })
  router.beforeEach(async (to) => {
    if (to.meta.public) return true
    if ((await signedIn()) !== 'no') return true
    return { name: 'sign-in', query: to.fullPath === '/' ? {} : { next: to.fullPath } }
  })
  return router
}
