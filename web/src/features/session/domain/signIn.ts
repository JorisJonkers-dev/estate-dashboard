/** The pages a browser needs no session for: there is nothing to return to on them. */
const ownPages = ['/sign-in', '/not-an-admin']

/** A backslash, which a browser reads as a slash, or a control character, which it drops. */
const rewrittenByABrowser = /[\\\u0000-\u001f\u007f]/

/**
 * Where signing in returns to: a page of this app, never another origin, and never sign-in itself.
 * `/\t/evil.test` is `//evil.test` by the time a browser follows it, so it is refused as that.
 */
export function safeNext(next: unknown): string {
  if (typeof next !== 'string') return '/'
  if (!next.startsWith('/') || next.startsWith('//') || rewrittenByABrowser.test(next)) return '/'
  if (next.startsWith('/auth/') || ownPages.some((page) => next === page || next.startsWith(`${page}?`))) return '/'
  return next
}

/** The server route that starts a sign-in through auth and comes back to next. */
export function signInHref(next: unknown): string {
  const to = safeNext(next)
  return to === '/' ? '/auth/login' : `/auth/login?next=${encodeURIComponent(to)}`
}
