import { getSession } from '../../../infrastructure/api/sdk.gen'

/** Whether an admin is signed in. `unknown` is a question nobody answered: the API was away. */
export type SignedIn = 'yes' | 'no' | 'unknown'

/**
 * Asks the API who is signed in. Only a 401 says nobody is: a request that failed, or an answer
 * the contract does not allow, is not a sign-out, and the page that follows reports it. The
 * generated client returns such a failure rather than throwing it.
 */
export async function whoIsSignedIn(): Promise<SignedIn> {
  const { data, response } = await getSession()
  if (response?.status === 401) return 'no'
  return data ? 'yes' : 'unknown'
}
