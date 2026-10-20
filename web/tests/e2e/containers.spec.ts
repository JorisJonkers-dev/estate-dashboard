import AxeBuilder from '@axe-core/playwright'
import { expect, type Page, test } from '@playwright/test'
import { mockFetch } from '../../src/infrastructure/mock'

/** Fails on any WCAG 2.1 A or AA violation axe finds on the page as it is now. */
async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

// The real binary serves the built app; its reads answer from the made-up estate, so every page
// has something to lay out.
test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const answer = await mockFetch(new Request(request.url(), { method: request.method() }))
    await route.fulfill({ status: answer.status, contentType: 'application/json', body: await answer.text() })
  })
})

test('opens the Containers list, a container’s page and a Project’s latest rollout, each accessible', async ({
  page,
}) => {
  await page.goto('/containers')
  await expect(page.getByRole('heading', { name: 'Containers', level: 1 })).toBeVisible()
  await expectAccessible(page)

  await page.getByRole('rowheader', { name: 'n8n' }).getByRole('link').click()
  await expect(page).toHaveURL('/containers/ghcr.io/jorisjonkers-dev/n8n')
  await expect(page.getByRole('heading', { name: 'Version history' })).toBeVisible()
  await expectAccessible(page)

  await page.goto('/projects/auth')
  await page.getByRole('link', { name: 'Latest rollout' }).click()
  await expect(page).toHaveURL('/projects/auth/rollout')
  await expect(page.getByRole('heading', { name: 'Order' })).toBeVisible()
  await expectAccessible(page)
})

test.describe('on a phone', () => {
  test.use({ viewport: { width: 390, height: 844 } })

  for (const path of [
    '/',
    '/projects',
    '/containers',
    '/containers/ghcr.io/jorisjonkers-dev/n8n',
    '/projects/auth',
    '/projects/auth/rollout',
    '/alerts',
    '/composition',
  ]) {
    test(`${path} never scrolls sideways at 390px`, async ({ page }) => {
      await page.goto(path)
      await page.waitForLoadState('networkidle')
      const { scroll, width } = await page.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        width: document.documentElement.clientWidth,
      }))
      expect(scroll).toBeLessThanOrEqual(width)
    })
  }
})
