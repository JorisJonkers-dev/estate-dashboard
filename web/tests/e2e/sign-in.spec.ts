import AxeBuilder from '@axe-core/playwright'
import { expect, type Page, test } from '@playwright/test'

/** Fails on any WCAG 2.1 A or AA violation axe finds on the page as it is now. */
async function expectAccessible(page: Page) {
  const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  expect(violations.map((v) => `${v.id}: ${v.help}`)).toEqual([])
}

/** Collects console errors: a Content-Security-Policy violation, a font that did not load, surface only there. */
function consoleErrors(page: Page): string[] {
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  return errors
}

/** The panel as the canvas draws it: its measures on a desktop, and on a phone. */
async function expectTheCanvasPanel(page: Page, maxWidth: string) {
  const phone = (page.viewportSize()?.width ?? 0) <= 480
  const panel = page.getByRole('main')
  await expect(panel).toHaveCSS('background-color', 'rgb(30, 30, 30)')
  await expect(panel).toHaveCSS('border-top-color', 'rgb(51, 51, 51)')
  await expect(panel).toHaveCSS('border-radius', '2px')
  await expect(panel).toHaveCSS('padding', phone ? '36px 24px' : '44px 48px')
  await expect(panel).toHaveCSS('font-size', '17px')
  await expect(page.getByRole('heading', { level: 1 })).toHaveCSS('font-size', phone ? '26px' : '30px')
  await expect(page.locator('.page')).toHaveCSS('background-color', 'rgb(20, 20, 20)')
  await expect(page.getByText('estate', { exact: true })).toHaveCSS('letter-spacing', '0.88px')

  const box = await panel.boundingBox()
  const viewport = page.viewportSize()
  if (!box || !viewport) throw new Error('the panel is not on the page')
  if (phone) {
    // 16px of page on either side, and the panel fills the rest.
    expect(Math.round(box.width)).toBe(viewport.width - 32)
  } else {
    expect(`${String(Math.round(box.width))}px`).toBe(maxWidth)
    // Centred both ways.
    expect(Math.abs(box.x + box.width / 2 - viewport.width / 2)).toBeLessThan(1)
    expect(Math.abs(box.y + box.height / 2 - viewport.height / 2)).toBeLessThan(1)
  }

  // The house face is served by the binary itself: nothing is fetched from another origin.
  await page.evaluate(() => document.fonts.ready)
  expect(await page.evaluate(() => document.fonts.check('600 22px "Barlow Semi Condensed"'))).toBe(true)
  const action = page.locator('.action')
  await expect(action).toHaveCSS('background-color', 'rgb(51, 135, 250)')
  await expect(action).toHaveCSS('min-height', '48px')
  expect((await action.boundingBox())?.width).toBe(box.width - (phone ? 50 : 98))
}

test('the sign-in page is the canvas page', async ({ page }) => {
  const errors = consoleErrors(page)
  const requests: string[] = []
  page.on('request', (request) => requests.push(new URL(request.url()).origin))
  await page.goto('/sign-in')

  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
  await expect(page.getByText("The estate's delivery dashboard, for admins. You sign in with your jorisjonkers.dev account.")).toBeVisible()
  await expect(page.getByRole('link', { name: 'Continue with jorisjonkers.dev' })).toHaveAttribute('href', '/auth/login')
  await expect(page.getByRole('link', { name: 'Forgot your password?' })).toHaveAttribute('href', 'https://auth.jorisjonkers.dev/forgot-password')
  await expect(page.getByRole('link', { name: 'jorisjonkers.dev', exact: true })).toHaveAttribute('href', 'https://jorisjonkers.dev')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expectTheCanvasPanel(page, '440px')
  await expectAccessible(page)
  expect(errors).toEqual([])
  expect([...new Set(requests)]).toEqual([new URL(page.url()).origin])
})

test('the sign-in page returns to the page that was asked for, and says when a sign-in failed', async ({ page }) => {
  await page.goto('/sign-in?next=%2F%3Fsince%3Dyesterday&failed=1')
  await expect(page.getByRole('link', { name: 'Continue with jorisjonkers.dev' })).toHaveAttribute('href', '/auth/login?next=%2F%3Fsince%3Dyesterday')
  await expect(page.getByRole('alert')).toHaveText('Signing in did not complete. Try again.')
  await expectAccessible(page)
})

test('the not-an-admin page is the canvas page', async ({ page }) => {
  const errors = consoleErrors(page)
  await page.goto('/not-an-admin')

  await expect(page.getByRole('heading', { name: 'This dashboard is for admins' })).toBeVisible()
  await expect(
    page.getByText('You are signed in, but your account does not hold the admin role. Ask an admin to grant it in auth, then sign in again.'),
  ).toBeVisible()
  await expect(page.getByRole('button', { name: 'Sign in as someone else' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Back to jorisjonkers.dev' })).toHaveAttribute('href', 'https://jorisjonkers.dev')
  await expectTheCanvasPanel(page, '480px')
  await expectAccessible(page)
  expect(errors).toEqual([])
})

test('signing in as someone else and signing in both go through the server', async ({ page }) => {
  // On a local run the server's sign-in routes are stand-ins that move the browser between the
  // app's own pages; what they do against auth is the Go suite's to prove.
  await page.goto('/not-an-admin')
  await page.getByRole('button', { name: 'Sign in as someone else' }).click()
  await expect(page).toHaveURL('/sign-in')
  await page.getByRole('link', { name: 'Continue with jorisjonkers.dev' }).click()
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { name: 'Alert history' })).toBeVisible()
})
