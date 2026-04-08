import { expect, test } from '@playwright/test'
import { mockPortalApi } from './mockApi'

test.beforeEach(async ({ page }) => {
  await mockPortalApi(page, { authenticated: true, role: 'admin' })
})

test('virtual keys workflow supports register and revoke', async ({ page }) => {
  await page.goto('/keys')

  await expect(page.getByRole('heading', { name: 'Virtual Keys' })).toBeVisible()

  await page.getByRole('button', { name: 'Add Key' }).click()
  await page.getByLabel('Provider').selectOption('google')
  await page.getByLabel('Display name').fill('Gemini Prod')
  await page.getByLabel('Real API key').fill('AIza-real-key')
  await page.getByRole('button', { name: 'Register' }).click()

  await expect(page.getByText('Key registered. Copy your virtual key now')).toBeVisible()
  const geminiRow = page.locator('tr', { hasText: 'Gemini Prod' }).first()
  await expect(geminiRow).toBeVisible()
  await expect(geminiRow).toContainText('Google Gemini')

  await geminiRow.getByRole('button', { name: 'Revoke' }).click()
  await page.getByRole('button', { name: 'Confirm' }).click()
  await expect(geminiRow).toContainText('Revoked')
})

test('policies workflow supports create, preview, and status updates', async ({ page }) => {
  await page.goto('/policies')

  await expect(page.getByRole('heading', { name: 'Policies' })).toBeVisible()

  await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Prod Guardrail')
  await page.getByRole('textbox', { name: 'Provider' }).first().fill('OPENAI')
  await page.getByRole('textbox', { name: 'Model Pattern' }).fill('GPT-4O')
  await page.getByRole('textbox', { name: 'Environment' }).first().fill('PRODUCTION')
  await page.getByRole('button', { name: 'Create Rule' }).click()

  await expect(page.getByText('Prod Guardrail').first()).toBeVisible()

  await page.getByRole('button', { name: 'Preview Policy Match' }).click()
  await expect(page.getByText('deny via Block prod secrets')).toBeVisible()

  await page.getByRole('button', { name: 'Pause' }).first().click()
  await expect(page.getByRole('button', { name: 'Resume' }).first()).toBeVisible()

  await page.getByRole('button', { name: 'Reviewing' }).first().click()
  await expect(page.getByText('reviewing', { exact: true }).first()).toBeVisible()
})

test('prompt release workflow supports promotion from release view', async ({ page }) => {
  await page.goto('/prompts/support-bot.system')

  await expect(page.getByRole('heading', { name: 'Prompt Release View' })).toBeVisible()

  await page.getByLabel('Environment').first().fill('production')
  await page.getByLabel('Version').selectOption('3')
  await page.getByLabel('Release Tag', { exact: true }).fill('2026.04-prod.1')
  await page.getByLabel('Notes').fill('promote after validation')
  await page.getByLabel('Promotion Reason').fill('reduce escalation hallucinations')
  await page.getByRole('button', { name: 'Promote Release' }).click()

  await expect(page.getByText('Promoted support-bot.system v3 to production as 2026.04-prod.1.')).toBeVisible()
})
