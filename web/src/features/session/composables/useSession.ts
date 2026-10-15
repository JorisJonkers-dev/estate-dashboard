import { getSession } from '../../../infrastructure/api/sdk.gen'

/** Whether an admin is signed in. `unknown` means the API did not answer. */
export type SignedIn = 'yes' | 'no' | 'unknown'

/**
 * Asks the API who is signed in. Only a 401 means nobody is. A failed request, or an answer the
 * contract does not allow, is not a sign-out, and the next page reports it. The generated client
 * returns such a failure rather than throwing it.
 */
export async function whoIsSignedIn(): Promise<SignedIn> {
  const { data, response } = await getSession()
  if (response?.status === 401) return 'no'
  return data ? 'yes' : 'unknown'
}
