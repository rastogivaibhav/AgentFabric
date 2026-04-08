import { expect, test } from '@playwright/test'
import { mockPortalApi } from './mockApi'

test('unauthenticated user is redirected to login and can sign in', async ({ page }) => {
  await mockPortalApi(page, { authenticated: false })

  await page.goto('/dashboard')
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()

  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByText('Username and password are required.')).toBeVisible()

  await page.getByLabel('Username').fill('admin')
  await page.getByLabel('Password').fill('admin')
  await page.getByRole('button', { name: 'Sign in' }).click()

  await expect(page).toHaveURL(/\/dashboard$/)
  await expect(page.getByRole('heading', { name: 'Your AI. Under control.' })).toBeVisible()
})

test('viewer role hides admin-only navigation areas', async ({ page }) => {
  await mockPortalApi(page, { authenticated: true, role: 'viewer' })

  await page.goto('/dashboard')
  await expect(page.getByRole('heading', { name: 'Your AI. Under control.' })).toBeVisible()

  await expect(page.getByText('ADMIN', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Policies' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Prompts' })).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Users' })).toHaveCount(0)

  await expect(page.getByRole('link', { name: 'Dashboard' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Traces' })).toBeVisible()
})
