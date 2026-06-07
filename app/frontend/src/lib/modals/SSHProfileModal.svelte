<script>
  // SSH profile manager: list + editor. Owns the form/editor/key-browse state
  // and persists via bindings; profile-list changes are reported to the parent
  // (which shares sshProfiles with the sidebar). Connecting and host-key
  // verification stay in the parent (terminal creation). Styles from the global stylesheets (styles/).
  import { SaveSSHProfile, UpdateSSHProfile, DeleteSSHProfile, SetSSHPassword, ListSSHKeys } from '../../../wailsjs/go/main/App';
  import { t } from '../i18n.js';

  export let profiles = [];
  export let secretBackend = '';
  export let connecting = false;
  export let onClose = () => {};
  export let onConnect = () => {};
  export let onProfilesChanged = () => {};
  export let onError = () => {};

  const emptyForm = () => ({
    name: '',
    host: '',
    port: 22,
    username: '',
    authMethod: 'password',
    keyPath: '',
    password: '',
    jumpHostEnabled: false,
    jumpHost: '',
    jumpPort: 22,
    jumpUsername: '',
    jumpAuthMethod: 'password',
    jumpKeyPath: '',
    jumpPassword: '',
    useTmux: true,
    rcMode: 'off',
    rcSnippet: '~/.bashrc',
  });

  let editingId = null; // null = list view, '__new__' = new profile, else profile id
  let form = emptyForm();
  let keys = [];

  function openEditor(profile) {
    if (profile) {
      editingId = profile.id;
      form = {
        name: profile.name,
        host: profile.host,
        port: profile.port,
        username: profile.username,
        authMethod: profile.authMethod,
        keyPath: profile.keyPath || '',
        password: '',
        jumpHostEnabled: Boolean(profile.jumpHostEnabled),
        jumpHost: profile.jumpHost || '',
        jumpPort: profile.jumpPort || 22,
        jumpUsername: profile.jumpUsername || '',
        jumpAuthMethod: profile.jumpAuthMethod || 'password',
        jumpKeyPath: profile.jumpKeyPath || '',
        jumpPassword: '',
        useTmux: profile.useTmux !== false,
        rcMode: profile.rcMode || 'off',
        rcSnippet: profile.rcSnippet || '~/.bashrc',
      };
    } else {
      editingId = '__new__';
      form = emptyForm();
    }
  }

  function cancelEdit() {
    editingId = null;
    keys = [];
  }

  async function browseKeys() {
    try {
      keys = await ListSSHKeys();
    } catch (e) {
      console.error('Failed to list SSH keys:', e);
      keys = [];
    }
  }

  function selectKey(keyPath) {
    form.keyPath = keyPath;
    keys = [];
  }

  async function save() {
    try {
      const profileData = {
        name: form.name,
        host: form.host,
        port: parseInt(form.port) || 22,
        username: form.username,
        authMethod: form.authMethod,
        keyPath: form.authMethod === 'key' ? form.keyPath : '',
        jumpHostEnabled: Boolean(form.jumpHostEnabled),
        jumpHost: form.jumpHostEnabled ? form.jumpHost : '',
        jumpPort: form.jumpHostEnabled ? (parseInt(form.jumpPort) || 22) : 22,
        jumpUsername: form.jumpHostEnabled ? form.jumpUsername : '',
        jumpAuthMethod: form.jumpHostEnabled ? form.jumpAuthMethod : 'password',
        jumpKeyPath: form.jumpHostEnabled && form.jumpAuthMethod === 'key' ? form.jumpKeyPath : '',
        useTmux: Boolean(form.useTmux),
        rcMode: form.rcMode || 'off',
        rcSnippet: form.rcMode === 'local-snippet' ? form.rcSnippet : '',
      };

      let updated;
      if (editingId && editingId !== '__new__') {
        profileData.id = editingId;
        updated = await UpdateSSHProfile(JSON.stringify(profileData));
      } else {
        updated = await SaveSSHProfile(JSON.stringify(profileData));
      }

      if (form.password) {
        const saved = updated.find((p) => p.name === profileData.name && p.host === profileData.host);
        if (saved) {
          await SetSSHPassword(saved.id, form.password);
        }
      }
      if (form.jumpPassword) {
        const saved = updated.find((p) => p.id === profileData.id) || updated.find((p) => p.name === profileData.name && p.host === profileData.host);
        if (saved) {
          await SetSSHPassword(`${saved.id}:jump`, form.jumpPassword);
        }
      }

      onProfilesChanged(updated);
      editingId = null;
      form = emptyForm();
    } catch (e) {
      onError(`Failed to save SSH profile: ${e.message || e}`);
    }
  }

  async function deleteProfile(id) {
    try {
      onProfilesChanged(await DeleteSSHProfile(id));
    } catch (e) {
      onError(`Failed to delete SSH profile: ${e.message || e}`);
    }
  }
</script>

<div class="modal-overlay" on:click|self={onClose} on:keydown={(e) => { if (e.key === 'Escape') onClose(); }} tabindex="0" role="button">
  <div class="ssh-modal">
    <div class="modal-header">
      <h3>{editingId ? (editingId === '__new__' ? $t('sshProfile.titleNew') : $t('sshProfile.titleEdit')) : $t('sshProfile.titleList')}</h3>
      <button class="header-btn close-btn" on:click={onClose}>✕</button>
    </div>

    {#if secretBackend === 'encrypted_file'}
      <div class="warning-banner">
        {$t('sshProfile.keyringWarning')}
      </div>
    {/if}

    {#if !editingId}
      <!-- Profile list view -->
      <div class="ssh-profile-list">
        {#if profiles.length === 0}
          <p class="sidebar-empty" style="padding: 1rem; text-align: center;">{$t('sshProfile.empty')}</p>
        {:else}
          {#each profiles as profile (profile.id)}
            <div class="ssh-profile-item">
              <div class="ssh-profile-info">
                <span class="ssh-profile-name">{profile.name}</span>
                <span class="ssh-profile-detail">{profile.username}@{profile.host}:{profile.port}</span>
                {#if profile.jumpHostEnabled}
                  <span class="ssh-profile-detail">{$t('sshProfile.jumpVia')} {profile.jumpUsername}@{profile.jumpHost}:{profile.jumpPort || 22}</span>
                {/if}
              </div>
              <div class="ssh-profile-actions">
                <button class="add-btn" on:click={() => onConnect(profile)} disabled={connecting}>
                  {connecting ? $t('sshProfile.connecting') : $t('sshProfile.connect')}
                </button>
                <button class="header-btn" on:click={() => openEditor(profile)} title={$t('sshProfile.edit')}>✎</button>
                <button class="header-btn close-btn" on:click={() => deleteProfile(profile.id)} title={$t('sshProfile.delete')}>✕</button>
              </div>
            </div>
          {/each}
        {/if}
      </div>
      <div style="padding: 0.75rem; border-top: 1px solid var(--border-subtle);">
        <button class="add-btn" on:click={() => openEditor(null)} style="width: 100%;">{$t('sshProfile.newProfile')}</button>
      </div>

    {:else}
      <!-- Profile editor view -->
      <div class="ssh-profile-editor">
        <label>
          <span>{$t('sshProfile.name')}</span>
          <input type="text" bind:value={form.name} placeholder={$t('sshProfile.namePlaceholder')} />
        </label>
        <label>
          <span>{$t('sshProfile.host')}</span>
          <input type="text" bind:value={form.host} placeholder="192.168.1.100" />
        </label>
        <label>
          <span>{$t('sshProfile.port')}</span>
          <input type="number" bind:value={form.port} min="1" max="65535" />
        </label>
        <label>
          <span>{$t('sshProfile.username')}</span>
          <input type="text" bind:value={form.username} placeholder="root" />
        </label>
        <label>
          <span>{$t('sshProfile.authMethod')}</span>
          <select bind:value={form.authMethod}>
            <option value="password">{$t('sshProfile.authPassword')}</option>
            <option value="key">{$t('sshProfile.authKey')}</option>
          </select>
        </label>

        {#if form.authMethod === 'password'}
          <label>
            <span>{$t('sshProfile.password')}</span>
            <input type="password" bind:value={form.password} placeholder={$t('sshProfile.passwordPlaceholder')} />
          </label>
        {:else}
          <label>
            <span>{$t('sshProfile.keyPath')}</span>
            <div style="display: flex; gap: 0.5rem;">
              <input type="text" bind:value={form.keyPath} placeholder="~/.ssh/id_ed25519" style="flex: 1;" />
              <button class="add-btn" on:click={browseKeys}>{$t('sshProfile.browse')}</button>
            </div>
          </label>
          {#if keys.length > 0}
            <div class="ssh-key-list">
              {#each keys as key (key.path)}
                <button class="ssh-key-item" on:click={() => selectKey(key.path)}>
                  <span class="ssh-key-name">{key.name}</span>
                  <span class="ssh-key-type">{key.type}</span>
                </button>
              {/each}
            </div>
          {/if}
          <label>
            <span>{$t('sshProfile.passphrase')}</span>
            <input type="password" bind:value={form.password} placeholder={$t('sshProfile.passphrasePlaceholder')} />
          </label>
        {/if}

        <div class="ssh-session-settings">
          <label class="ssh-toggle-row">
            <input type="checkbox" bind:checked={form.jumpHostEnabled} />
            <span>{$t('sshProfile.useJumpHost')}</span>
          </label>
          {#if form.jumpHostEnabled}
            <label>
              <span>{$t('sshProfile.jumpHost')}</span>
              <input type="text" bind:value={form.jumpHost} placeholder="bastion.example.com" />
            </label>
            <label>
              <span>{$t('sshProfile.jumpPort')}</span>
              <input type="number" bind:value={form.jumpPort} min="1" max="65535" />
            </label>
            <label>
              <span>{$t('sshProfile.jumpUsername')}</span>
              <input type="text" bind:value={form.jumpUsername} placeholder="jump-user" />
            </label>
            <label>
              <span>{$t('sshProfile.jumpAuthMethod')}</span>
              <select bind:value={form.jumpAuthMethod}>
                <option value="password">{$t('sshProfile.authPassword')}</option>
                <option value="key">{$t('sshProfile.authKey')}</option>
              </select>
            </label>
            {#if form.jumpAuthMethod === 'password'}
              <label>
                <span>{$t('sshProfile.jumpPassword')}</span>
                <input type="password" bind:value={form.jumpPassword} placeholder={$t('sshProfile.passwordPlaceholder')} />
              </label>
            {:else}
              <label>
                <span>{$t('sshProfile.jumpKeyPath')}</span>
                <input type="text" bind:value={form.jumpKeyPath} placeholder="~/.ssh/id_ed25519" />
              </label>
              <label>
                <span>{$t('sshProfile.jumpPassphrase')}</span>
                <input type="password" bind:value={form.jumpPassword} placeholder={$t('sshProfile.passphrasePlaceholder')} />
              </label>
            {/if}
          {/if}
        </div>

        <div class="ssh-session-settings">
          <label class="ssh-toggle-row">
            <input type="checkbox" bind:checked={form.useTmux} />
            <span>{$t('sshProfile.useTmux')}</span>
          </label>
          <label>
            <span>{$t('sshProfile.rcMode')}</span>
            <select bind:value={form.rcMode}>
              <option value="off">{$t('sshProfile.rcModes.off')}</option>
              <option value="remote-default">{$t('sshProfile.rcModes.remoteDefault')}</option>
              <option value="mimir">{$t('sshProfile.rcModes.mimir')}</option>
              <option value="local-snippet">{$t('sshProfile.rcModes.localSnippet')}</option>
            </select>
          </label>
          {#if form.rcMode === 'local-snippet'}
            <label>
              <span>{$t('sshProfile.rcPath')}</span>
              <input type="text" bind:value={form.rcSnippet} placeholder="~/.bashrc" />
            </label>
          {/if}
          <p>{$t('sshProfile.rcNote')}</p>
        </div>

        <div class="ssh-editor-actions">
          <button class="header-btn" on:click={cancelEdit}>{$t('sshProfile.cancel')}</button>
          <button class="add-btn" on:click={save}>{$t('sshProfile.save')}</button>
        </div>
      </div>
    {/if}
  </div>
</div>
