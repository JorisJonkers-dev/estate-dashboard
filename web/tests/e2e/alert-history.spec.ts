import AxeBuilder from '@axe-core/playwright'
import { expect, type Page, test } from '@playwright/test'

/** Fails on any WCAG 2.1 A or AA violation axe finds on the page as it is now. */
async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

test('the history is read from the migrated database through the real API', async ({ page }) => {
  // A Content-Security-Policy violation surfaces only as a console error, so any error fails.
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  const answered = page.waitForResponse((response) => response.url().endsWith('/api/v1/alerts/history') && response.status() === 200)
  await page.goto('/')
  await answered
  await expect(page.getByRole('heading', { name: 'Alert history' })).toBeVisible()
  // Nothing writes the history until the dashboard reads Alertmanager, so the table is empty.
  await expect(page.getByText('No alert has been recorded yet.')).toBeVisible()
  await expectAccessible(page)
  expect(errors).toEqual([])
})

test('a client route is served the app, and an unknown one goes home', async ({ page }) => {
  await page.goto('/no/such/page')
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { name: 'Alert history' })).toBeVisible()
})
