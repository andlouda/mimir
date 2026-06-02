<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { t } from './i18n.js';
  import { SearchCommandHistory, GetHistoryStats, GetHistoryHostnames, DeleteHistoryEntry, PurgeHistoryBefore } from '../../wailsjs/go/main/App';

  const dispatch = createEventDispatcher();

  export let activeTerminalId = null;

  let entries = [];
  let total = 0;
  let searchQuery = '';
  let selectedHost = '';
  let selectedType = '';
  let selectedExitFilter = '';
  let selectedTimeRange = '7d';
  let hostnames = [];
  let selectedEntry = null;
  let showStats = false;
  let stats = null;
  let loading = false;
  let page = 0;
  const pageSize = 50;

  let searchTimeout = null;

  $: sinceTimestamp = computeSince(selectedTimeRange);

  function computeSince(range) {
    const now = new Date();
    switch (range) {
      case '1h': now.setHours(now.getHours() - 1); break;
      case '24h': now.setDate(now.getDate() - 1); break;
      case '7d': now.setDate(now.getDate() - 7); break;
      case '30d': now.setDate(now.getDate() - 30); break;
      case 'all': return '';
      default: now.setDate(now.getDate() - 7);
    }
    return now.toISOString();
  }

  async function search() {
    loading = true;
    try {
      const params = {
        query: searchQuery,
        hostname: selectedHost,
        sessionType: selectedType,
        since: sinceTimestamp,
        limit: pageSize,
        offset: page * pageSize
      };
      if (selectedExitFilter === 'success') params.exitCode = 0;
      else if (selectedExitFilter === 'failed') params.failedOnly = true;

      const resultJSON = await SearchCommandHistory(JSON.stringify(params));
      const result = JSON.parse(resultJSON);
      if (result.error) {
        console.error('History search error:', result.error);
        entries = [];
        total = 0;
      } else {
        entries = result.entries || [];
        total = result.total || 0;
      }
    } catch (err) {
      console.error('History search failed:', err);
      entries = [];
      total = 0;
    }
    loading = false;
  }

  async function loadHostnames() {
    try {
      hostnames = await GetHistoryHostnames();
    } catch (err) {
      hostnames = [];
    }
  }

  async function loadStats() {
    try {
      const resultJSON = await GetHistoryStats(sinceTimestamp);
      stats = JSON.parse(resultJSON);
    } catch (err) {
      stats = null;
    }
  }

  function debouncedSearch() {
    clearTimeout(searchTimeout);
    page = 0;
    searchTimeout = setTimeout(search, 300);
  }

  function selectEntry(entry) {
    selectedEntry = selectedEntry?.id === entry.id ? null : entry;
  }

  async function deleteEntry(id) {
    try {
      await DeleteHistoryEntry(id);
      if (selectedEntry?.id === id) selectedEntry = null;
      await search();
    } catch (err) {
      console.error('Delete failed:', err);
    }
  }

  function runAgain(command) {
    if (activeTerminalId != null) {
      dispatch('runcommand', { terminalId: activeTerminalId, command });
    }
  }

  function formatTime(isoString) {
    if (!isoString) return '';
    try {
      const d = new Date(isoString);
      const now = new Date();
      const diffMs = now - d;
      const diffMin = Math.floor(diffMs / 60000);
      const diffHour = Math.floor(diffMs / 3600000);
      const diffDay = Math.floor(diffMs / 86400000);

      if (diffMin < 1) return 'just now';
      if (diffMin < 60) return `${diffMin}m ago`;
      if (diffHour < 24) return `${diffHour}h ago`;
      if (diffDay < 7) return `${diffDay}d ago`;
      return d.toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: '2-digit' });
    } catch {
      return isoString;
    }
  }

  function truncateCmd(cmd, maxLen = 60) {
    if (!cmd) return '';
    if (cmd.length <= maxLen) return cmd;
    return cmd.slice(0, maxLen) + '...';
  }

  function prevPage() {
    if (page > 0) { page--; search(); }
  }
  function nextPage() {
    if ((page + 1) * pageSize < total) { page++; search(); }
  }

  // Reactive: re-search when any filter changes (Svelte tracks the variable reads)
  let mounted = false;
  $: filterKey = `${selectedHost}|${selectedType}|${selectedExitFilter}|${selectedTimeRange}`;
  $: if (mounted && filterKey) {
    page = 0;
    search();
  }

  onMount(async () => {
    await loadHostnames();
    await search();
    mounted = true;
  });
</script>

<div class="history-dashboard">
  <div class="history-header">
    <div class="history-header-left">
      <button class="back-btn" on:click={() => dispatch('backToTerminals')}>
        &#x2190;
      </button>
      <h2>{$t('historyDashboard.title')}</h2>
      <span class="history-count">{$t('historyDashboard.count', { n: total })}</span>
    </div>
    <div class="history-header-right">
      <button class="stats-btn" class:active={showStats} on:click={() => { showStats = !showStats; if (showStats) loadStats(); }}>
        {$t('historyDashboard.stats')}
      </button>
      <button class="refresh-btn" on:click={() => { search(); loadHostnames(); }}>
        &#x21BB;
      </button>
    </div>
  </div>

  <div class="history-filters">
    <div class="search-box">
      <span class="search-icon-h">&#x2315;</span>
      <input
        type="text"
        bind:value={searchQuery}
        on:input={debouncedSearch}
        placeholder={$t('historyDashboard.searchPlaceholder')}
        class="search-input"
      />
    </div>
    <div class="filter-row">
      <select bind:value={selectedHost} class="filter-select">
        <option value="">{$t('historyDashboard.allHosts')}</option>
        {#each hostnames as h}
          <option value={h}>{h}</option>
        {/each}
      </select>
      <select bind:value={selectedType} class="filter-select">
        <option value="">{$t('historyDashboard.allTypes')}</option>
        <option value="bash">{$t('historyDashboard.typeBash')}</option>
        <option value="zsh">{$t('historyDashboard.typeZsh')}</option>
        <option value="ssh">{$t('historyDashboard.typeSsh')}</option>
      </select>
      <select bind:value={selectedExitFilter} class="filter-select">
        <option value="">{$t('historyDashboard.allExits')}</option>
        <option value="success">{$t('historyDashboard.exitSuccess')}</option>
        <option value="failed">{$t('historyDashboard.exitFailed')}</option>
      </select>
      <select bind:value={selectedTimeRange} class="filter-select">
        <option value="1h">{$t('historyDashboard.range1h')}</option>
        <option value="24h">{$t('historyDashboard.range24h')}</option>
        <option value="7d">{$t('historyDashboard.range7d')}</option>
        <option value="30d">{$t('historyDashboard.range30d')}</option>
        <option value="all">{$t('historyDashboard.rangeAll')}</option>
      </select>
    </div>
  </div>

  <div class="history-body">
    <div class="history-list" class:has-detail={selectedEntry}>
      {#if loading}
        <div class="loading-state">{$t('historyDashboard.loading')}</div>
      {:else if entries.length === 0}
        <div class="empty-state-h">{$t('historyDashboard.noCommands')}</div>
      {:else}
        {#each entries as entry (entry.id)}
          <button
            class="history-row"
            class:selected={selectedEntry?.id === entry.id}
            class:failed={entry.exitCode !== 0}
            on:click={() => selectEntry(entry)}
          >
            <span class="cmd-text">{truncateCmd(entry.command)}</span>
            <span class="cmd-meta">
              <span class="exit-badge" class:exit-ok={entry.exitCode === 0} class:exit-fail={entry.exitCode !== 0}>
                {entry.exitCode}
              </span>
              {#if entry.hostname}
                <span class="host-tag">{entry.hostname}</span>
              {/if}
              <span class="time-tag">{formatTime(entry.startedAt)}</span>
            </span>
          </button>
        {/each}
        <div class="pagination">
          <button disabled={page === 0} on:click={prevPage}>{$t('historyDashboard.prev')}</button>
          <span>{$t('historyDashboard.page', { n: page + 1, total: Math.max(1, Math.ceil(total / pageSize)) })}</span>
          <button disabled={(page + 1) * pageSize >= total} on:click={nextPage}>{$t('historyDashboard.next')}</button>
        </div>
      {/if}
    </div>

    {#if selectedEntry}
      <div class="history-detail">
        <div class="detail-header">
          <h3>{$t('historyDashboard.details')}</h3>
          <button class="detail-close" on:click={() => selectedEntry = null}>&#x2715;</button>
        </div>
        <div class="detail-fields">
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.command')}</span>
            <pre class="detail-value cmd-pre">{selectedEntry.command}</pre>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.exitCode')}</span>
            <span class="detail-value">
              <span class="exit-badge" class:exit-ok={selectedEntry.exitCode === 0} class:exit-fail={selectedEntry.exitCode !== 0}>
                {selectedEntry.exitCode}
              </span>
            </span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.directory')}</span>
            <span class="detail-value">{selectedEntry.cwd || '-'}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.host')}</span>
            <span class="detail-value">{selectedEntry.hostname || 'local'}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.user')}</span>
            <span class="detail-value">{selectedEntry.username || '-'}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.shell')}</span>
            <span class="detail-value">{selectedEntry.shell || '-'}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.type')}</span>
            <span class="detail-value">{selectedEntry.sessionType || '-'}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.session')}</span>
            <span class="detail-value">{selectedEntry.sessionId ?? '-'}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">{$t('historyDashboard.time')}</span>
            <span class="detail-value">{selectedEntry.startedAt || '-'}</span>
          </div>
        </div>
        <div class="detail-actions">
          {#if activeTerminalId != null}
            <button class="action-btn run-btn" on:click={() => runAgain(selectedEntry.command)}>
              {$t('historyDashboard.runAgain')}
            </button>
          {/if}
          <button class="action-btn delete-btn" on:click={() => deleteEntry(selectedEntry.id)}>
            {$t('historyDashboard.delete')}
          </button>
        </div>
      </div>
    {/if}
  </div>

  {#if showStats && stats}
    <div class="stats-panel">
      <div class="stats-grid">
        <div class="stat-card">
          <span class="stat-number">{stats.totalCommands}</span>
          <span class="stat-label">{$t('historyDashboard.totalCommands')}</span>
        </div>
        <div class="stat-card">
          <span class="stat-number fail-num">{stats.failedCount}</span>
          <span class="stat-label">{$t('historyDashboard.failed')}</span>
        </div>
        <div class="stat-card">
          <span class="stat-number">{stats.totalCommands > 0 ? ((1 - stats.failedCount / stats.totalCommands) * 100).toFixed(1) : 0}%</span>
          <span class="stat-label">{$t('historyDashboard.successRate')}</span>
        </div>
        <div class="stat-card">
          <span class="stat-number">{stats.hostBreakdown?.length || 0}</span>
          <span class="stat-label">{$t('historyDashboard.hosts')}</span>
        </div>
      </div>

      <div class="stats-columns">
        <div class="stats-col">
          <h4>{$t('historyDashboard.topCommands')}</h4>
          {#if stats.topCommands?.length}
            {#each stats.topCommands.slice(0, 8) as tc}
              <div class="stat-bar-row">
                <span class="stat-bar-label">{tc.label}</span>
                <div class="stat-bar">
                  <div class="stat-bar-fill" style="width: {Math.min(100, (tc.count / (stats.topCommands[0]?.count || 1)) * 100)}%"></div>
                </div>
                <span class="stat-bar-count">{tc.count}</span>
              </div>
            {/each}
          {:else}
            <span class="stat-empty">{$t('historyDashboard.noData')}</span>
          {/if}
        </div>
        <div class="stats-col">
          <h4>{$t('historyDashboard.topDirectories')}</h4>
          {#if stats.topDirs?.length}
            {#each stats.topDirs.slice(0, 8) as td}
              <div class="stat-bar-row">
                <span class="stat-bar-label" title={td.label}>{td.label.split('/').pop() || td.label}</span>
                <div class="stat-bar">
                  <div class="stat-bar-fill dir-fill" style="width: {Math.min(100, (td.count / (stats.topDirs[0]?.count || 1)) * 100)}%"></div>
                </div>
                <span class="stat-bar-count">{td.count}</span>
              </div>
            {/each}
          {:else}
            <span class="stat-empty">{$t('historyDashboard.noData')}</span>
          {/if}
        </div>
        <div class="stats-col">
          <h4>{$t('historyDashboard.commandsPerDay')}</h4>
          {#if stats.perDay?.length}
            <div class="sparkline">
              {#each stats.perDay as pd}
                <div
                  class="spark-bar"
                  style="height: {Math.max(4, (pd.count / Math.max(...stats.perDay.map(d => d.count))) * 60)}px"
                  title="{pd.date}: {pd.count}"
                ></div>
              {/each}
            </div>
          {:else}
            <span class="stat-empty">{$t('historyDashboard.noData')}</span>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>
