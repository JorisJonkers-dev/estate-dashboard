import * as z from 'zod'

// The CSP forbids eval. Imported first, before any schema is built, so zod never probes for it.
z.config({ jitless: true })
