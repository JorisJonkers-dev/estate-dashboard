import './jitless'
import { client } from './api/client.gen'

/** Points the generated client at this page's origin. The Go binary serves the API too. */
export function configureApi(options: { baseUrl?: string; fetch?: typeof fetch } = {}): void {
  client.setConfig({ baseUrl: options.baseUrl ?? '', ...(options.fetch ? { fetch: options.fetch } : {}) })
}
