import { expect, test } from '@playwright/test';
import { installMimirMocks } from './fixtures/mimirApp.js';

// A saved session with four panes: one local shell whose start is rate
// limited once, one that never starts, one SSH pane whose profile is gone
// and one plain shell. The app must bring back what it can, retry the
// rate-limited start, and report exactly the failed entries so the
// backend keeps them for the next start.
test('restore retries rate limits, keeps failed panes for next time and reports them', async ({ page }) => {
  await installMimirMocks(page);
  await page.addInitScript(() => {
    const saved = [
      { type: 'bash', name: 'limited', resumeId: 'r-limited' },
      { type: 'bash', name: 'broken', resumeId: 'r-broken' },
      { type: 'ssh', name: 'SSH: gone', sshProfileId: 'missing-profile', resumeId: 'r-ssh' },
      { type: 'zsh', name: 'plain', resumeId: 'r-plain' },
    ];
    const starts = [];
    let next = 10;
    let limitedOnce = false;
    const start = async (type) => {
      // The restore does not pass the name; count by order instead.
      const n = starts.push(type);
      if (n === 1 && !limitedOnce) { limitedOnce = true; throw new Error('rate limit exceeded for start_terminal, try again shortly'); }
      if (n === 3) throw new Error('boom: pty unavailable');
      return next++;
    };
    window.__restoreReport = null;
    window.__starts = starts;
    const install = () => {
      const app = window.go?.main?.App;
      if (!app) return false;
      app.GetLoadedSessionData = async () => ({ terminals: saved, layout: '' });
      app.StartTerminalWithOptions = start;
      app.StartTerminal = start;
      app.FinishSessionRestore = async (json) => { window.__restoreReport = JSON.parse(json); };
      return true;
    };
    if (!install()) {
      const timer = setInterval(() => { if (install()) clearInterval(timer); }, 5);
    }
  });
  await page.goto('/');
  await expect(page.locator('.brand-text')).toHaveText('Mimir');

  // Two panes come back (the rate-limited one after its retry, ~2.5 s).
  await expect(page.locator('.terminal-container')).toHaveCount(2, { timeout: 15000 });
  await expect(page.getByText('limited', { exact: true })).toBeVisible();
  await expect(page.getByText('plain', { exact: true })).toBeVisible();

  await expect.poll(() => page.evaluate(() => window.__restoreReport), { timeout: 15000 }).not.toBeNull();
  const report = await page.evaluate(() => window.__restoreReport);
  expect(report.map((t) => t.name).sort()).toEqual(['SSH: gone', 'broken']);

  const banner = page.locator('.error-message').first();
  await expect(banner).toContainText('broken');
  await expect(banner).toContainText('SSH profile no longer exists');
  await expect(banner).toContainText('tried again at the next start');
});
