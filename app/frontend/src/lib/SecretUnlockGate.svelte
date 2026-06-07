<script>
  import { onMount } from 'svelte';
  import { t } from './i18n.js';
  import {
    SecretStoreState,
    SetupSecretMasterPassword,
    UnlockSecrets,
    ListFIDOChallenges,
  } from '../../wailsjs/go/main/App';

  // Called once the credential store is usable (keyring, unlocked, or N/A).
  let { onresolved = () => {} } = $props();

  const MIN_LEN = 8;

  // phase: 'loading' | 'setup' | 'unlock' | 'done'
  let phase = $state('loading');
  let password = $state('');
  let confirm = $state('');
  let error = $state('');
  let busy = $state(false);
  let fidoAvailable = $state(false);

  onMount(refreshState);

  async function refreshState() {
    phase = 'loading';
    error = '';
    let state = 'unavailable';
    try {
      state = await SecretStoreState();
    } catch (e) {
      // If we cannot even query the store, do not hard-block the app; the
      // backend will still reject secret operations until it is usable.
      resolve();
      return;
    }

    switch (state) {
      case 'needs_setup':
        phase = 'setup';
        break;
      case 'locked':
        phase = 'unlock';
        await detectFido();
        break;
      // 'keyring' | 'unlocked' | 'unavailable'
      default:
        resolve();
    }
  }

  async function detectFido() {
    try {
      const challenges = await ListFIDOChallenges();
      fidoAvailable = Array.isArray(challenges) && challenges.length > 0;
    } catch {
      fidoAvailable = false;
    }
  }

  function resolve() {
    phase = 'done';
    password = '';
    confirm = '';
    onresolved();
  }

  async function submitSetup() {
    error = '';
    if (password.length < MIN_LEN) {
      error = $t('secretGate.errors.minLength', { min: MIN_LEN });
      return;
    }
    if (password !== confirm) {
      error = $t('secretGate.errors.mismatch');
      return;
    }
    busy = true;
    try {
      await SetupSecretMasterPassword(password);
      resolve();
    } catch (e) {
      error = friendly(e);
    } finally {
      busy = false;
    }
  }

  async function submitUnlock() {
    error = '';
    if (!password) {
      error = $t('secretGate.errors.enterPassword');
      return;
    }
    busy = true;
    try {
      await UnlockSecrets(password);
      resolve();
    } catch (e) {
      error = friendly(e);
    } finally {
      busy = false;
    }
  }

  function unlockWithSecurityKey() {
    // The FIDO2 unlock ceremony (obtaining the authenticator hmac-secret/PRF
    // output) is pending the architecture decision in ADR-0011. The crypto and
    // bindings (ListFIDOChallenges / UnlockSecretsFIDO / EnrollFIDO) already
    // exist; this button activates once a ceremony provider ships.
    error = $t('secretGate.errors.fidoUnavailable');
  }

  function friendly(e) {
    const msg = (e && e.toString) ? e.toString() : String(e);
    if (msg.includes('incorrect master password')) return $t('secretGate.errors.incorrect');
    if (msg.includes('already initialized')) return $t('secretGate.errors.alreadySet');
    if (msg.includes('at least')) return $t('secretGate.errors.minLength', { min: MIN_LEN });
    return msg.replace(/^Error:\s*/, '');
  }

  function onKey(event, fn) {
    if (event.key === 'Enter') fn();
  }
</script>

{#if phase === 'loading' || phase === 'setup' || phase === 'unlock'}
  <div class="gate-overlay" role="dialog" aria-modal="true" aria-label={$t('secretGate.ariaUnlock')}>
    <div class="gate-card">
      <div class="gate-brand">mimir</div>

      {#if phase === 'loading'}
        <p class="gate-sub">{$t('secretGate.checking')}</p>
      {:else if phase === 'setup'}
        <h2>{$t('secretGate.setupTitle')}</h2>
        <p class="gate-sub">{$t('secretGate.setupBody')}</p>
        <input
          type="password"
          placeholder={$t('secretGate.masterPassword')}
          bind:value={password}
          autocomplete="new-password"
          disabled={busy}
        />
        <input
          type="password"
          placeholder={$t('secretGate.confirm')}
          bind:value={confirm}
          autocomplete="new-password"
          disabled={busy}
          onkeydown={(e) => onKey(e, submitSetup)}
        />
        <button class="gate-primary" onclick={submitSetup} disabled={busy}>
          {busy ? $t('secretGate.creating') : $t('secretGate.create')}
        </button>
      {:else if phase === 'unlock'}
        <h2>{$t('secretGate.unlockTitle')}</h2>
        <p class="gate-sub">{$t('secretGate.unlockBody')}</p>
        <input
          type="password"
          placeholder={$t('secretGate.masterPassword')}
          bind:value={password}
          autocomplete="current-password"
          disabled={busy}
          onkeydown={(e) => onKey(e, submitUnlock)}
        />
        <button class="gate-primary" onclick={submitUnlock} disabled={busy}>
          {busy ? $t('secretGate.unlocking') : $t('secretGate.unlock')}
        </button>
        {#if fidoAvailable}
          <button class="gate-secondary" onclick={unlockWithSecurityKey} disabled={busy}>
            {$t('secretGate.useSecurityKey')}
          </button>
        {/if}
      {/if}

      {#if error}
        <p class="gate-error" role="alert">{error}</p>
      {/if}
    </div>
  </div>
{/if}

<style>
  .gate-overlay {
    position: fixed;
    inset: 0;
    z-index: 9999;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(11, 16, 24, 0.92);
    backdrop-filter: blur(4px);
  }
  .gate-card {
    width: 360px;
    max-width: 90vw;
    background: #1b2636;
    border: 1px solid #2c3a52;
    border-radius: 10px;
    padding: 28px 26px;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
    display: flex;
    flex-direction: column;
    gap: 12px;
    color: #e6edf3;
    font-family: system-ui, -apple-system, sans-serif;
  }
  .gate-brand {
    font-weight: 700;
    letter-spacing: 0.5px;
    opacity: 0.7;
    font-size: 14px;
  }
  .gate-card h2 {
    margin: 0;
    font-size: 18px;
  }
  .gate-sub {
    margin: 0;
    font-size: 13px;
    line-height: 1.5;
    color: #9fb0c3;
  }
  .gate-card input {
    background: #0e1622;
    border: 1px solid #2c3a52;
    border-radius: 6px;
    padding: 10px 12px;
    color: #e6edf3;
    font-size: 14px;
    outline: none;
  }
  .gate-card input:focus {
    border-color: #4c8bf5;
  }
  .gate-primary {
    background: #4c8bf5;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 10px 12px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
  }
  .gate-primary:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .gate-secondary {
    background: transparent;
    color: #9fb0c3;
    border: 1px solid #2c3a52;
    border-radius: 6px;
    padding: 9px 12px;
    font-size: 13px;
    cursor: pointer;
  }
  .gate-error {
    margin: 0;
    color: #ff6b6b;
    font-size: 13px;
  }
</style>
