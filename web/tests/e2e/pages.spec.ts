import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import { mockFetch } from '../../src/infrastructure/mock'

// The real binary serves the built app; its reads answer from the made-up estate, so each page is
// checked with everything it can show on it.
test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const answer = await mockFetch(new Request(request.url(), { method: request.method() }))
    await route.fulfill({ status: answer.status, contentType: 'application/json', body: await answer.text() })
  })
})

for (const [path, heading] of [
  ['/', 'Overview'],
  ['/projects', 'Projects'],
  ['/alerts', 'Alerts'],
  ['/composition', 'Composition'],
  ['/projects/auth', 'auth'],
] as const) {
  test(`${path} has no WCAG 2.1 A or AA violation`, async ({ page }) => {
    await page.goto(path)
    await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible()
    await page.waitForLoadState('networkidle')
    const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
    expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([])
  })
}
