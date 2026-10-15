import { getSession } from '../../../infrastructure/api/sdk.gen'

/** Whether an admin is signed in. `unknown` means the API did not answer. */
export type SignedIn = 'yes' | 'no' | 'unknown'

/** Asks the API who is signed in. Only a 401 means nobody is. */
export async function whoIsSignedIn(): Promise<SignedIn> {
  const { data, response } = await getSession()
  if (response?.status === 401) return 'no'
  return data ? 'yes' : 'unknown'
}
