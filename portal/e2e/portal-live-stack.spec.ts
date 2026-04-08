import { expect, test } from '@playwright/test'

test.describe('live stack smoke', () => {
  test('core pages render from live Docker stack', async ({ page }) => {
    await page.goto('/dashboard')
    await expect(page.getByRole('heading', { name: 'Your AI. Under control.' })).toBeVisible()
    await expect(page.getByText('LIVE ACTIVITY FEED')).toBeVisible()

    await page.goto('/traces')
    await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible()

    await page.goto('/policies')
    await expect(page.getByRole('heading', { name: 'Policies' })).toBeVisible()

    await page.goto('/prompts')
    await expect(page.getByRole('heading', { name: 'Prompt Registry' })).toBeVisible()

    await page.goto('/keys')
    await expect(page.getByRole('heading', { name: 'Virtual Keys' })).toBeVisible()

    await page.goto('/evals')
    await expect(page.getByRole('heading', { name: 'Evaluations' })).toBeVisible()
  })

  test('trace compare route handles missing selections safely', async ({ page }) => {
    await page.goto('/traces/compare')
    await expect(page.getByText('Choose two traces from the traces page to compare them.')).toBeVisible()
  })
})
