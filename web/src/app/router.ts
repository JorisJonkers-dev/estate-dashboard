import { createRouter, createWebHistory, type Router, type RouterHistory } from 'vue-router'
import type { SignedIn } from '../features/session/composables/useSession'

declare module 'vue-router' {
  interface RouteMeta {
    /** A page that needs no session. It draws its own frame. */
    public?: boolean
  }
}

/** Asks the API who is signed in. Loads on demand, after main.ts sets up zod. */
async function askTheApi(): Promise<SignedIn> {
  const { whoIsSignedIn } = await import('../features/session/composables/useSession')
  return whoIsSignedIn()
}

/** The app's routes. Every page but the two public ones needs a signed-in admin. */
export function createAppRouter(
  history: RouterHistory = createWebHistory(),
  signedIn: () => Promise<SignedIn> = askTheApi,
): Router {
  const router = createRouter({
    history,
    routes: [
      { path: '/', name: 'overview', component: () => import('../features/overview/components/OverviewPage.vue') },
      {
        path: '/projects',
        name: 'projects',
        component: () => import('../features/projects/components/ProjectsPage.vue'),
      },
      {
        path: '/projects/:name',
        name: 'project',
        component: () => import('../features/projects/components/ProjectPage.vue'),
      },
      {
        path: '/projects/:name/release',
        name: 'release',
        component: () => import('../features/projects/components/ReleasePage.vue'),
      },
      { path: '/alerts', name: 'alerts', component: () => import('../features/alerts/components/AlertsPage.vue') },
      {
        path: '/alerts/history',
        name: 'alert-history',
        component: () => import('../features/alerts/components/AlertHistoryPage.vue'),
      },
      {
        path: '/composition',
        name: 'composition',
        component: () => import('../features/composition/components/CompositionPage.vue'),
      },
      {
        path: '/sign-in',
        name: 'sign-in',
        meta: { public: true },
        component: () => import('../features/session/components/SignInPage.vue'),
      },
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
