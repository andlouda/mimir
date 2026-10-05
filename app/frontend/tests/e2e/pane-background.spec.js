import { expect, test } from '@playwright/test';
import { openApp } from './fixtures/mimirApp.js';

const IMAGE_ID = '0123456789abcdef.png';
// 1×1 PNG, served for /mimir-bg/<id> in place of the Go asset handler.
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64');

test('a pane gets a background image from the context menu and can drop it again', async ({ page }) => {
  await page.route('**/mimir-bg/*', (route) => route.fulfill({ status: 200, contentType: 'image/png', body: PNG }));
  await openApp(page);
  await page.evaluate((id) => {
    window.go.main.App.ListTerminalBackgroundsJSON = async () => JSON.stringify([{ id, name: 'nebula.png', size: 20480, addedAt: '2026-10-05T10:00:00Z' }]);
    window.__bgUpdates = [];
    window.go.main.App.UpdateTerminalBackground = async (termId, json) => { window.__bgUpdates.push([termId, json]); };
  }, IMAGE_ID);

  const pane = page.locator('.terminal-container').first();
  await pane.click({ button: 'right' });
  await page.getByRole('menuitem', { name: 'Background…' }).click();
  const dialog = page.getByRole('dialog', { name: /Pane background/ });
  await expect(dialog).toBeVisible();

  await dialog.getByRole('option', { name: 'nebula.png' }).click();
  const layer = page.locator('.pane-background');
  await expect(layer).toHaveCount(1);
  await expect(layer).toHaveCSS('background-image', new RegExp(`/mimir-bg/${IMAGE_ID}`));
  await expect(layer).toHaveCSS('opacity', '0.35');

  await dialog.getByLabel('Fit').selectOption('tile');
  await expect(layer).toHaveCSS('background-repeat', 'repeat');
  await page.screenshot({ path: process.env.BG_SHOT || 'test-results/pane-background.png' });

  // Stored with the pane: the last update carries the current settings.
  const updates = await page.evaluate(() => window.__bgUpdates);
  expect(JSON.parse(updates.at(-1)[1])).toMatchObject({ id: IMAGE_ID, fit: 'tile', opacity: 0.35 });

  await dialog.getByRole('option', { name: 'No image' }).click();
  await expect(layer).toHaveCount(0);
  expect((await page.evaluate(() => window.__bgUpdates)).at(-1)[1]).toBe('');

  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
});
