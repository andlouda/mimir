<script>
  // Timeline tab of the agent panel: tool calls with their ids, the
  // processes and connections attributed to them, and the agent's own
  // telemetry, as one list per session. Data comes from the timeline store
  // (initial load) and arrives live in batches while the tab is open.
  import { onDestroy } from 'svelte';
  import { EventsOn } from '../../wailsjs/runtime';
  import { t } from './i18n.js';
  import { applyFilters, buildItems, clockText, confidenceLabel, durationText, mergeBatch, summarize, CATEGORY } from './agents/timeline.js';
  import { copyAgentTimelinePath, loadAgentTimeline, openAgentTimelineFolder } from './actions/agentActions.js';

  export let terminalId;
  /** Claude hook status of the agent's host (null until loaded). */
  export let hook = null;
  export let hookBusy = false;
  export let onInstallHook = () => {};
  export let onFeedback = () => {};

  const INITIAL_LIMIT = 500;

  let events = [];
  let enabled = true;
  let sessionId = '';
  let path = '';
  let loading = false;
  let error = '';
  let loadedFor = null;
  let off = null;
  let expanded = new Set();
  let filters = { [CATEGORY.tools]: true, [CATEGORY.processes]: true, [CATEGORY.network]: true, [CATEGORY.api]: true, [CATEGORY.session]: true };

  $: if (terminalId !== loadedFor) { loadedFor = terminalId; subscribe(); load(); }
  $: items = applyFilters(buildItems(events), filters);
  $: needsHookUpdate = enabled && hook && hook.installed && !hook.toolEvents && hook.toolEventsWanted;
  $: noHook = enabled && hook && !hook.installed;

  async function load() {
    loading = true;
    error = '';
    try {
      const res = await loadAgentTimeline(terminalId, 0, INITIAL_LIMIT);
      enabled = res.enabled !== false;
      sessionId = res.sessionId || '';
      path = res.path || '';
      events = mergeBatch([], res.events || []);
    } catch (e) {
      error = String(e?.message || e);
    } finally {
      loading = false;
    }
  }

  function subscribe() {
    if (off) { off(); off = null; }
    events = [];
    expanded = new Set();
    off = EventsOn(`agent-timeline-${terminalId}`, (raw) => {
      try {
        const batch = JSON.parse(raw);
        if (batch?.sessionId && sessionId && batch.sessionId !== sessionId) {
          // The pane moved to another session: start over.
          sessionId = batch.sessionId;
          events = [];
          path = '';
        }
        if (!sessionId && batch?.sessionId) sessionId = batch.sessionId;
        enabled = true;
        events = mergeBatch(events, batch?.events || []);
      } catch { /* ignore malformed batch */ }
    });
  }

  function toggle(key) {
    const next = new Set(expanded);
    if (next.has(key)) next.delete(key); else next.add(key);
    expanded = next;
  }

  function toggleFilter(cat) {
    filters = { ...filters, [cat]: !filters[cat] };
  }

  async function copyPath() {
    try {
      const p = await copyAgentTimelinePath(terminalId);
      onFeedback(p ? $t('agentPanel.timeline.pathCopied') : $t('agentPanel.timeline.noFile'));
    } catch (e) { onFeedback(String(e?.message || e)); }
  }

  async function openFolder() {
    try { await openAgentTimelineFolder(); } catch (e) { onFeedback(String(e?.message || e)); }
  }

  function statusMark(item) {
    if (item.kind !== 'tool') return '';
    if (item.status === 'failed') return '✗';
    if (item.status === 'ok') return '✓';
    if (item.status === 'running') return '…';
    return '';
  }

  function typeLabel(ev) {
    return `${ev.source}/${ev.type}`;
  }

  onDestroy(() => { if (off) off(); });
</script>

<div class="agent-section-head">
  <span>{$t('agentPanel.timeline.title')}{sessionId ? ` · ${sessionId.slice(0, 8)}` : ''}</span>
  <span class="agent-ops">
    <button type="button" class="agent-link" on:click={load} disabled={loading}>{loading ? '…' : $t('agentPanel.gitReload')}</button>
    <button type="button" class="agent-link" on:click={copyPath} disabled={!path} title={path}>{$t('agentPanel.timeline.copyPath')}</button>
    <button type="button" class="agent-link" on:click={openFolder}>{$t('agentPanel.timeline.openFolder')}</button>
  </span>
</div>

{#if error}
  <p class="agent-panel-error">{error}</p>
{:else if !enabled}
  <p class="agent-panel-hint">{$t('agentPanel.timeline.disabledHint')}</p>
{:else}
  {#if noHook}
    <p class="agent-panel-hint agent-panel-warn">
      <span>{$t('agentPanel.timeline.noHookHint')}</span>
      <button type="button" class="agent-btn" on:click={onInstallHook} disabled={hookBusy}>{hookBusy ? '…' : $t('agentPanel.hookInstall')}</button>
    </p>
  {:else if needsHookUpdate}
    <p class="agent-panel-hint agent-panel-warn">
      <span>{$t('agentPanel.timeline.hookUpdateHint')}</span>
      <button type="button" class="agent-btn" on:click={onInstallHook} disabled={hookBusy}>{hookBusy ? '…' : $t('agentPanel.hookUpdate')}</button>
    </p>
  {/if}
  <div class="tl-filters" role="group" aria-label={$t('agentPanel.timeline.filters')}>
    {#each Object.values(CATEGORY) as cat (cat)}
      <button type="button" class="tl-chip {filters[cat] ? 'tl-chip-on' : ''}" aria-pressed={filters[cat]} on:click={() => toggleFilter(cat)}>{$t(`agentPanel.timeline.cat_${cat}`)}</button>
    {/each}
  </div>
  {#if items.length === 0}
    <p class="agent-panel-hint">{loading ? $t('agentPanel.loading') : $t('agentPanel.timeline.empty')}</p>
  {/if}
  {#each items as item (item.key)}
    {#if item.kind === 'tool'}
      <div class="tl-item tl-tool {item.status === 'failed' ? 'tl-failed' : ''}">
        <button type="button" class="tl-row tl-head" aria-expanded={expanded.has(item.key)} on:click={() => toggle(item.key)}>
          <span class="tl-time">{clockText(item.ts)}</span>
          <span class="tl-mark">{statusMark(item)}</span>
          <span class="tl-tool-name">{item.tool || '?'}</span>
          <code class="tl-summary">{item.pre ? summarize(item.pre) : (item.post ? summarize(item.post) : '')}</code>
          <span class="tl-meta">
            {#if item.durationMs != null}{durationText(item.durationMs)}{/if}
            {#if item.children?.length} · {item.children.length} {$t('agentPanel.timeline.subagents')}{/if}
            {#if item.processes.length} · {item.processes.length} {$t('agentPanel.timeline.procs')}{/if}
            {#if item.connections.length} · {item.connections.length} {$t('agentPanel.timeline.conns')}{/if}
            {#if item.anomalies.length} · <span class="tl-anomaly">{item.anomalies.map((a) => $t(`agentPanel.timeline.anomaly_${a}`)).join(', ')}</span>{/if}
          </span>
        </button>
        {#if expanded.has(item.key)}
          <div class="tl-children">
            {#each item.children || [] as child (child.key)}
              <div class="tl-row tl-child tl-subagent" title={child.toolUseId}>
                <span class="tl-time">{clockText(child.ts)}</span>
                <span class="tl-mark">{statusMark(child)}</span>
                <span class="tl-tool-name">{child.tool || '?'}</span>
                <code class="tl-summary">{child.pre ? summarize(child.pre) : ''}</code>
                <span class="tl-meta">{$t('agentPanel.timeline.subagent')}{#if child.durationMs != null} · {durationText(child.durationMs)}{/if}{#if child.processes.length} · {child.processes.length} {$t('agentPanel.timeline.procs')}{/if}</span>
              </div>
            {/each}
            <div class="tl-row tl-child tl-dim"><span class="tl-time"></span><span class="tl-type">tool_use_id</span><code class="tl-summary">{item.toolUseId}</code></div>
            {#each item.events as ev (ev.seq)}
              <div class="tl-row tl-child" title={JSON.stringify(ev.attrs || {}, null, 1)}>
                <span class="tl-time">{clockText(ev.ts)}</span>
                <span class="tl-type">{typeLabel(ev)}</span>
                <code class="tl-summary">{summarize(ev)}</code>
                <span class="tl-meta">
                  {#if ev.pid}pid {ev.pid}{/if}
                  {#if ev.confidence} <span class="tl-conf" title={$t(`agentPanel.timeline.conf_${ev.confidence}`)}>{confidenceLabel(ev)}</span>{/if}
                  {#if ev.redacted} · {$t('agentPanel.timeline.redacted')}{/if}
                  {#if ev.anomaly} · <span class="tl-anomaly">{$t(`agentPanel.timeline.anomaly_${ev.anomaly}`)}</span>{/if}
                </span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {:else}
      {#each item.events as ev (ev.seq)}
        <div class="tl-item tl-row tl-single" title={JSON.stringify(ev.attrs || {}, null, 1)}>
          <span class="tl-time">{clockText(ev.ts)}</span>
          <span class="tl-type">{typeLabel(ev)}</span>
          <code class="tl-summary">{summarize(ev)}</code>
          <span class="tl-meta">
            {#if ev.pid}pid {ev.pid}{/if}
            {#if ev.confidence} <span class="tl-conf" title={$t(`agentPanel.timeline.conf_${ev.confidence}`)}>{confidenceLabel(ev)}</span>{/if}
            {#if ev.anomaly} · <span class="tl-anomaly">{$t(`agentPanel.timeline.anomaly_${ev.anomaly}`)}</span>{/if}
          </span>
        </div>
      {/each}
    {/if}
  {/each}
  <p class="agent-panel-hint">{$t('agentPanel.timeline.hint')}</p>
{/if}

<style>
  .tl-filters { display: flex; flex-wrap: wrap; gap: 4px; padding: 2px 0 6px; }
  .tl-chip { background: transparent; border: 1px solid var(--border-subtle); border-radius: 10px; color: var(--text-secondary); font-size: 10px; padding: 1px 8px; cursor: pointer; }
  .tl-chip-on { color: inherit; border-color: #63b3ed; }
  .tl-item { border: 1px solid var(--border-subtle); border-radius: 6px; margin-bottom: 4px; font-size: 11px; }
  .tl-failed { border-color: rgba(248, 81, 73, 0.4); }
  .tl-row { display: flex; align-items: baseline; gap: 8px; padding: 3px 8px; min-width: 0; }
  .tl-head { width: 100%; background: transparent; border: 0; color: inherit; text-align: left; cursor: pointer; font: inherit; }
  .tl-head:hover { background: var(--bg-elevated, rgba(255,255,255,0.04)); }
  .tl-time { color: var(--text-secondary); font-family: var(--font-mono); font-size: 10px; min-width: 5.5em; flex-shrink: 0; }
  .tl-mark { min-width: 1em; flex-shrink: 0; }
  .tl-tool-name { font-weight: 600; flex-shrink: 0; }
  .tl-type { color: var(--text-secondary); font-family: var(--font-mono); font-size: 10px; min-width: 9em; flex-shrink: 0; }
  .tl-summary { font-family: var(--font-mono); font-size: 11px; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tl-meta { color: var(--text-secondary); font-size: 10px; flex-shrink: 0; white-space: nowrap; }
  .tl-children { border-top: 1px solid var(--border-subtle); padding: 2px 0; }
  .tl-child { padding-left: 18px; }
  .tl-dim { color: var(--text-secondary); }
  .tl-anomaly { color: #f6ad55; }
  .tl-conf { font-family: var(--font-mono); letter-spacing: -1px; }
  .tl-subagent { border-left: 2px solid var(--border-subtle); margin-left: 18px; padding-left: 8px; }
</style>
