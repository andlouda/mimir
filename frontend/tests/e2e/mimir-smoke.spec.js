import { expect, test } from '@playwright/test';
import { mockPlaybooks, openApp } from './fixtures/mimirApp.js';

test('starts with a mocked local terminal and primary navigation', async ({ page }) => {
  await openApp(page);

  await expect(page.getByText('Terminal')).toBeVisible();
  await expect(page.getByText('SSH Hosts')).toBeVisible();
  await expect(page.getByRole('button', { name: '+ New' })).toBeVisible();
  await expect(page.getByText(/1 active/)).toBeVisible();
  await expect(page.locator('#terminal-1')).toBeVisible();
});

test('opens workflow playbooks and can inspect a protected playbook in the builder', async ({ page }) => {
  await openApp(page);

  await page.getByText('Workflow').click();
  await expect(page.getByRole('heading', { name: 'Workflow Builder' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Troubleshooting Playbooks' })).toBeVisible();
  await expect(page.getByText(mockPlaybooks[0].name)).toBeVisible();
  await expect(page.getByText(mockPlaybooks[1].name)).toBeVisible();

  await page.getByRole('button', { name: 'Open in Builder' }).first().click();
  await expect(page.getByLabel('Name')).toHaveValue(mockPlaybooks[0].name);
  await expect(page.getByRole('button', { name: 'Save as Copy' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Workflow Steps' })).toBeVisible();
});

test('saves a protected playbook as a copy after editing it', async ({ page }) => {
  await openApp(page);

  await page.getByText('Workflow').click();
  await page.getByRole('button', { name: 'Open in Builder' }).first().click();
  await expect(page.getByRole('button', { name: 'Save as Copy' })).toBeVisible();

  await page.getByLabel('Description').fill('Copied smoke-test playbook description.');
  await page.getByRole('button', { name: 'Save as Copy' }).click();

  await expect(page.getByText(`${mockPlaybooks[0].name} saved as a playbook.`)).toBeVisible();
  await expect(page.getByText('docker-compose-debug-copy')).toBeVisible();
});

test('runs a playbook through the workflow summary path', async ({ page }) => {
  await openApp(page);

  await page.getByText('Workflow').click();
  await page.locator('.playbook-card', { hasText: mockPlaybooks[0].name }).getByRole('button', { name: 'Run' }).click();

  await expect(page.getByText('Latest run available')).toBeVisible();
  await expect(page.getByText('2 values')).toBeVisible();
  await expect(page.getByText('Discovery completed.')).toBeVisible();
});

test('shows pending approval and continues after approval', async ({ page }) => {
  await openApp(page);

  await page.getByText('Workflow').click();
  await page.locator('.playbook-card', { hasText: 'Approval Drill' }).getByRole('button', { name: 'Run' }).click();

  const approvalDialog = page.locator('.approval-modal');
  await expect(approvalDialog).toBeVisible();
  await expect(approvalDialog.getByRole('heading', { name: 'Approve Workflow Step' })).toBeVisible();
  await expect(approvalDialog.getByText('Restart Service').first()).toBeVisible();
  await expect(approvalDialog.getByText('high-risk tools require approval')).toBeVisible();

  await page.getByRole('button', { name: 'Approve and Continue' }).click();
  await expect(page.getByText('Step approved and workflow continued.')).toBeVisible();
  await expect(page.getByText('Latest run available')).toBeVisible();
});

test('shows pending approval and stops cleanly after denial', async ({ page }) => {
  await openApp(page);

  await page.getByText('Workflow').click();
  await page.locator('.playbook-card', { hasText: 'Approval Drill' }).getByRole('button', { name: 'Run' }).click();

  await expect(page.locator('.approval-modal')).toBeVisible();
  await page.getByRole('button', { name: 'Deny' }).click();

  await expect(page.getByText('Step denied. The workflow remained stopped.')).toBeVisible();
  await expect(page.getByText('approval denied by user')).toBeVisible();
});

test('opens the transcript viewer from the terminal header', async ({ page }) => {
  await openApp(page);

  await page.locator('.terminal-header .transcript-btn').first().click();

  await expect(page.getByRole('heading', { name: 'Terminal Transcripts' })).toBeVisible();
  await expect(page.getByText('mocked transcript body — first line')).toBeVisible();

  // The list is collapsed by default when opened from a specific terminal;
  // toggle "Browse" to see the other transcripts.
  await expect(page.getByRole('button', { name: 'API host' })).not.toBeVisible();
  await page.getByRole('button', { name: /Browse \(\d+\)/ }).click();
  await expect(page.getByRole('button', { name: 'API host' })).toBeVisible();

  await page.getByRole('button', { name: 'Local shell' }).click();
  // Selecting a transcript auto-collapses the list again.
  await expect(page.getByRole('button', { name: 'API host' })).not.toBeVisible();
  await expect(page.getByText('mocked transcript body — first line')).toBeVisible();

  await page.locator('.transcript-viewer').getByRole('button', { name: 'Close' }).first().click();
  await expect(page.getByRole('heading', { name: 'Terminal Transcripts' })).not.toBeVisible();
});

test('settings page exposes release and history smoke surfaces', async ({ page }) => {
  await openApp(page);

  await page.getByText('Settings').click();
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
  await expect(page.getByText('Updates')).toBeVisible();
  await expect(page.getByRole('button', { name: /Command History/ })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Check' })).toBeVisible();

  await page.getByRole('button', { name: 'Check' }).click();
  await expect(page.getByText('Update Status')).toBeVisible();
  await expect(page.getByText(/Current: 0\.2\.0 .* Latest: 0\.2\.0/)).toBeVisible();
});
