import { describe, expect, it } from 'vitest'
import { safeNext, signInHref } from './signIn'

describe('safeNext', () => {
  it('keeps a page of this app, with its query', () => {
    expect(safeNext('/alerts?page=2')).toBe('/alerts?page=2')
    expect(safeNext('/sign-in-history')).toBe('/sign-in-history')
  })

  it.each([
    ['nothing', undefined],
    ['a list, as a repeated query parameter arrives', ['/alerts', '/other']],
    ['another origin', 'https://evil.test/alerts'],
    ['a scheme-relative address', '//evil.test'],
    ['a backslash a browser reads as a slash', '/\\evil.test'],
    ['a tab a browser drops', '/\t/evil.test'],
    ['a newline a browser drops', '/\n/evil.test'],
    ['a delete character', '/a\u007fb'],
    ['a relative path', 'alerts'],
    ['a sign-in route of the server', '/auth/logout'],
    ['the sign-in page', '/sign-in'],
    ['the sign-in page, with a query', '/sign-in?failed=1'],
    ['the not-an-admin page', '/not-an-admin'],
  ])('goes home for %s', (_, next) => {
    expect(safeNext(next)).toBe('/')
  })
})

describe('signInHref', () => {
  it('starts a sign-in that returns home', () => {
    expect(signInHref(undefined)).toBe('/auth/login')
    expect(signInHref('https://evil.test')).toBe('/auth/login')
  })

  it('carries the page to return to, encoded', () => {
    expect(signInHref('/alerts?page=2&q=a b')).toBe('/auth/login?next=%2Falerts%3Fpage%3D2%26q%3Da%20b')
  })
})
