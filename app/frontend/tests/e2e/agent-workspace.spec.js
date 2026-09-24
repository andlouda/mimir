import { expect, test } from '@playwright/test';
import { installMimirMocks } from './fixtures/mimirApp.js';

async function openWorkspace(page, options) {
  await installMimirMocks(page, options);
  await page.goto('/');
  await page.getByRole('button', { name: 'Agent workspace', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Agent workspace' })).toBeVisible();
}

async function createProjectAndTask(page, status = 'in_progress') {
  await page.getByRole('button', { name: 'New project', exact: true }).click();
  await page.getByLabel('Name', { exact: true }).fill('Mimir');
  await page.getByLabel('Project directory (optional)').fill('/repo');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.locator('.project-button', { hasText: 'Mimir' })).toBeVisible();
  await page.getByRole('button', { name: /^Tasks / }).click();
  await page.getByRole('button', { name: 'New task', exact: true }).click();
  await page.getByLabel('Task title').fill('Fix SSH reconnect');
  await page.getByLabel('Description / notes').fill('Verify reconnect behavior and retain the session.');
  await page.getByLabel('Task status').selectOption(status);
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByTestId('workspace-task')).toContainText('Fix SSH reconnect');
}

test('projects and tasks persist, feed attention and can be archived and restored', async ({ page }) => {
  await openWorkspace(page);
  await createProjectAndTask(page, 'review');
  await page.getByRole('button', { name: /^Needs attention / }).click();
  await expect(page.locator('.attention-card')).toContainText('Fix SSH reconnect');
  await page.reload();
  await page.getByRole('button', { name: 'Agent workspace', exact: true }).click();
  await expect(page.locator('.attention-card')).toContainText('Fix SSH reconnect');
  await page.locator('.project-button', { hasText: 'Mimir' }).click();
  await page.getByRole('button', { name: 'Edit project' }).click();
  await page.getByLabel('Archived', { exact: true }).check();
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.locator('.attention-card')).toHaveCount(0);
  await page.getByLabel('Show archived').check();
  await page.locator('.project-button', { hasText: 'Mimir' }).click();
  await page.getByRole('button', { name: 'Edit project' }).click();
  await page.getByLabel('Archived', { exact: true }).uncheck();
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.locator('.attention-card')).toContainText('Fix SSH reconnect');
  await page.getByRole('button', { name: 'Edit task' }).click();
  await page.getByLabel('Task status').selectOption('done');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.locator('.attention-card')).toHaveCount(0);
});

test('assigns discovered sessions and preserves them when the agent is gone', async ({ page }) => {
  await openWorkspace(page, { workspaceAgent: true });
  await createProjectAndTask(page);
  await expect(page.locator('.sidebar-agent-row')).toHaveCount(1);
  await page.evaluate(() => window.__emitMimir('agent-state-1', { state: 'permission', sessionFile: '/sessions/one.jsonl', prompt: 'permission_prompt', lastText: 'Allow the requested operation?' }));
  await page.getByTestId('workspace-task').getByRole('button', { name: 'Assign session' }).click();
  await page.getByLabel('Agent terminal').selectOption('1');
  await expect(page.getByRole('combobox', { name: 'Session', exact: true })).toHaveValue('/sessions/one.jsonl');
  await page.getByRole('button', { name: 'Assign', exact: true }).click();
  await expect(page.getByTestId('workspace-session')).toContainText('Fix SSH reconnect');
  await expect(page.getByTestId('workspace-session')).toContainText('Open in terminal');
  await page.getByRole('button', { name: /^Needs attention / }).click();
  await expect(page.locator('.attention-card')).toContainText('Approval requested');
  await expect(page.locator('.attention-card')).toContainText('Fix SSH reconnect');
  await page.evaluate(() => localStorage.setItem('test-agent-offline', 'yes'));
  await page.reload();
  await page.getByRole('button', { name: 'Agent workspace', exact: true }).click();
  await page.getByRole('button', { name: /^Sessions / }).click();
  await expect(page.getByTestId('workspace-session')).toContainText('No connected terminal');
  await page.getByRole('button', { name: /^Tasks / }).click();
  await expect(page.getByTestId('workspace-task')).toContainText('In progress');
  await page.getByRole('button', { name: /^Sessions / }).click();
  await page.getByRole('button', { name: 'Remove assignment' }).click();
  await expect(page.getByTestId('workspace-session')).toHaveCount(0);
  const source = await page.evaluate(async () => JSON.parse(await window.go.main.App.ListAgentSessionsJSON()));
  expect(source.sessions).toHaveLength(1);
});

test('a save failure leaves the form and existing tasks intact', async ({ page }) => {
  await openWorkspace(page);
  await createProjectAndTask(page);
  await page.getByRole('button', { name: 'Edit task' }).click();
  await page.getByLabel('Task title').fill('Unsaved change');
  await page.evaluate(() => { window.go.main.App.SaveAgentWorkspaceTaskJSON = async () => { throw new Error('disk full'); }; });
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByRole('alert')).toHaveText('disk full');
  await expect(page.getByLabel('Task title')).toHaveValue('Unsaved change');
  await expect(page.getByTestId('workspace-task')).toContainText('Fix SSH reconnect');
});
