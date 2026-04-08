import { expect, test } from '@playwright/test'
import { mockPortalApi } from './mockApi'

test.beforeEach(async ({ page }) => {
  await mockPortalApi(page, { authenticated: true, role: 'admin' })
})

test('dashboard renders control plane overview and core navigation', async ({ page }) => {
  await page.goto('/dashboard')

  await expect(page.getByRole('heading', { name: 'Your AI. Under control.' })).toBeVisible()
  await expect(page.getByText('LIVE ACTIVITY FEED')).toBeVisible()
  await expect(page.getByText('ACTIVE ROLLOUTS', { exact: true })).toBeVisible()
  await expect(page.getByText('BUDGET STATUS', { exact: true })).toBeVisible()

  await page.goto('/evals')
  await expect(page.getByRole('heading', { name: 'Evaluations' })).toBeVisible()

  await page.goto('/prompts')
  await expect(page.getByRole('heading', { name: 'Prompt Registry' })).toBeVisible()
  await expect(page.getByText('support-bot.system').first()).toBeVisible()
})

test('traces flow supports selecting two traces and opening compare view', async ({ page }) => {
  await page.goto('/traces')

  await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible()

  const compareButton = page.getByRole('button', { name: 'Compare selected' })
  await expect(compareButton).toBeDisabled()

  const checkboxes = page.locator('tbody input[type="checkbox"]')
  await expect(checkboxes).toHaveCount(3)

  await checkboxes.nth(0).check()
  await checkboxes.nth(1).check()
  await expect(compareButton).toBeEnabled()

  await compareButton.click()
  await expect(page).toHaveURL(/\/traces\/compare\?left=trace-left&right=trace-right$/)
  await expect(page.getByRole('heading', { name: 'Trace Comparison' })).toBeVisible()
  await expect(page.getByText('candidate trace is slower')).toBeVisible()
  await expect(page.getByText('STATUS', { exact: true })).toBeVisible()
})
