import * as z from 'zod'
import { client } from './api/client.gen'

/** Points the generated client at this page's origin. The Go binary serves the API too. */
export function configureApi(options: { baseUrl?: string; fetch?: typeof fetch } = {}): void {
  // The CSP forbids eval, so zod must not probe for it.
  z.config({ jitless: true })
  client.setConfig({ baseUrl: options.baseUrl ?? '', ...(options.fetch ? { fetch: options.fetch } : {}) })
}
