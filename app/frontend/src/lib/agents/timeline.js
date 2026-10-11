// Pure helpers for the agent timeline tab: turning the normalised events
// (see docs/agent-timeline.md) into rows and tool-call groups, merging live
// batches, and the text shown per event. No DOM, no stores.

export const RING_MAX = 5000;

/** Event categories for the filter chips. */
export const CATEGORY = { tools: 'tools', processes: 'processes', network: 'network', api: 'api', session: 'session' };

export function categoryOf(ev) {
  switch (ev.source) {
    case 'proc': return CATEGORY.processes;
    case 'net': return CATEGORY.network;
    case 'otel':
      return ev.type === 'tool_result' || ev.type === 'tool_decision' ? CATEGORY.tools : CATEGORY.api;
    case 'hook':
    case 'transcript':
      return ev.type === 'tool_pre' || ev.type === 'tool_post' || ev.type === 'tool_use' || ev.type === 'tool_result' ? CATEGORY.tools : CATEGORY.session;
    default:
      return CATEGORY.session;
  }
}

/**
 * Merges a live batch into the known events: deduped by seq, kept in seq
 * order, bounded to RING_MAX.
 */
export function mergeBatch(existing, batch) {
  const bySeq = new Map();
  for (const ev of existing || []) bySeq.set(ev.seq, ev);
  for (const ev of batch || []) bySeq.set(ev.seq, ev);
  const out = [...bySeq.values()].sort((a, b) => a.seq - b.seq);
  return out.length > RING_MAX ? out.slice(out.length - RING_MAX) : out;
}

function toolInput(ev) {
  const input = ev.attrs?.tool_input;
  return input && typeof input === 'object' ? input : null;
}

/** One line of text for an event: what it did, redacted as stored. */
export function summarize(ev) {
  const a = ev.attrs || {};
  const input = toolInput(ev);
  switch (ev.type) {
    case 'tool_pre':
    case 'tool_post':
    case 'tool_use':
      if (input) {
        if (typeof input.command === 'string') return input.command;
        if (typeof input.file_path === 'string') return input.file_path;
        if (typeof input.pattern === 'string') return input.pattern + (input.path ? ` in ${input.path}` : '');
        if (typeof input.url === 'string') return input.url;
        if (typeof input.description === 'string') return input.description;
        if (typeof input.prompt === 'string') return input.prompt;
        if (typeof input.skill === 'string') return input.skill;
      }
      return ev.tool_name || '';
    case 'tool_result':
      return [a.success === false || a.success === 'false' ? 'failed' : 'ok', a.duration_ms != null ? `${a.duration_ms} ms` : ''].filter(Boolean).join(' · ');
    case 'tool_decision':
      return [a.decision, a.source || a.decision_source].filter(Boolean).join(' · ');
    case 'api_request':
      return [a.model, a.duration_ms != null ? `${a.duration_ms} ms` : '', a.cost_usd != null ? `$${Number(a.cost_usd).toFixed(4)}` : ''].filter(Boolean).join(' · ');
    case 'api_error':
      return [a.model, a.status_code, a.error].filter(Boolean).join(' · ');
    case 'session_start':
      return [a.start_source, a.cwd].filter(Boolean).join(' · ');
    case 'notification':
      return [a.notification_type, a.message].filter(Boolean).join(' · ');
    case 'subagent_stop':
      return a.agent_id || '';
    case 'process_start':
    case 'process_end':
      return a.cmdline || a.exe || (ev.pid ? `pid ${ev.pid}` : '');
    case 'conn_open':
    case 'conn_close':
      return [a.proto, a.remote, a.state].filter(Boolean).join(' · ');
    default:
      return a.reason || a.message || '';
  }
}

/** Short badge for the confidence of an attributed runtime event. */
export function confidenceLabel(ev) {
  switch (ev.confidence) {
    case 'strong': return '●●●';
    case 'medium': return '●●○';
    case 'weak': return '●○○';
    case 'none': return '○○○';
    default: return '';
  }
}

function ms(ts) {
  const t = Date.parse(ts || '');
  return Number.isFinite(t) ? t : 0;
}

/**
 * Groups events into timeline items, newest first. A tool call (by
 * tool_use_id) is one item carrying its pre/post hook events, telemetry
 * result and decision, attributed processes and connections. Everything
 * else is a single-event item.
 * @returns {Array<{kind:'tool'|'event', key:string, ts:string, toolUseId?:string, tool?:string, events:object[], pre?:object, post?:object, result?:object, decision?:object, processes:object[], connections:object[], anomalies:string[], durationMs?:number, status:'running'|'ok'|'failed'|''}>}
 */
export function buildItems(events) {
  const groups = new Map();
  const items = [];
  for (const ev of events || []) {
    if (ev.tool_use_id) {
      let g = groups.get(ev.tool_use_id);
      if (!g) {
        g = { kind: 'tool', key: `tool:${ev.tool_use_id}`, ts: ev.ts, toolUseId: ev.tool_use_id, tool: ev.tool_name || '', events: [], processes: [], connections: [], anomalies: [], status: '' };
        groups.set(ev.tool_use_id, g);
        items.push(g);
      }
      g.events.push(ev);
      if (!g.tool && ev.tool_name) g.tool = ev.tool_name;
      if (ms(ev.ts) && (!g.ts || ms(ev.ts) < ms(g.ts))) g.ts = ev.ts;
      if (ev.type === 'tool_pre' || ev.type === 'tool_use') g.pre = g.pre || ev;
      else if (ev.type === 'tool_post') g.post = ev;
      else if (ev.type === 'tool_result') g.result = ev;
      else if (ev.type === 'tool_decision') g.decision = ev;
      else if (ev.source === 'proc') g.processes.push(ev);
      else if (ev.source === 'net') g.connections.push(ev);
      if (ev.anomaly && !g.anomalies.includes(ev.anomaly)) g.anomalies.push(ev.anomaly);
      continue;
    }
    items.push({ kind: 'event', key: `ev:${ev.seq}`, ts: ev.ts, events: [ev], processes: [], connections: [], anomalies: ev.anomaly ? [ev.anomaly] : [], status: '' });
  }
  for (const g of items) {
    if (g.kind !== 'tool') continue;
    const end = g.post || g.result;
    const failed = (g.post?.attrs?.response?.is_error === true) || g.result?.attrs?.success === false || g.result?.attrs?.success === 'false';
    g.status = failed ? 'failed' : end ? 'ok' : (g.pre ? 'running' : '');
    if (g.pre && end && ms(g.pre.ts) && ms(end.ts) >= ms(g.pre.ts)) g.durationMs = ms(end.ts) - ms(g.pre.ts);
    else if (g.result?.attrs?.duration_ms != null) g.durationMs = Number(g.result.attrs.duration_ms);
  }
  return nestSubagents(items).sort((a, b) => ms(b.ts) - ms(a.ts) || (b.events[0]?.seq || 0) - (a.events[0]?.seq || 0));
}

/**
 * Subagent calls (Claude Code's tracing beta) carry spans whose parent span
 * belongs to the parent's Task/Agent tool call. Such groups move under
 * their parent as `children`, newest first, one level deep.
 */
export function nestSubagents(items) {
  const groupBySpan = new Map();
  for (const it of items) {
    if (it.kind !== 'tool') continue;
    for (const ev of it.events) {
      if (ev.source === 'otel' && ev.attrs?.span_id) groupBySpan.set(ev.attrs.span_id, it);
    }
  }
  if (!groupBySpan.size) return items;
  const nested = new Set();
  for (const it of items) {
    if (it.kind !== 'tool') continue;
    const parentSpan = it.events.find((ev) => ev.type === 'span_tool' && ev.attrs?.parent_span_id)?.attrs.parent_span_id;
    const parent = parentSpan ? groupBySpan.get(parentSpan) : null;
    if (!parent || parent === it) continue;
    parent.children = parent.children || [];
    parent.children.push(it);
    nested.add(it);
  }
  for (const it of items) {
    if (it.children) it.children.sort((a, b) => ms(b.ts) - ms(a.ts));
  }
  return items.filter((it) => !nested.has(it));
}

/** Keeps the items whose category is enabled; tool groups always count as tools. */
export function applyFilters(items, enabled) {
  return (items || []).filter((it) => {
    if (it.kind === 'tool') return enabled[CATEGORY.tools] !== false;
    return enabled[categoryOf(it.events[0])] !== false;
  });
}

/** "10:42:01" (local time) from an ISO timestamp. */
export function clockText(ts) {
  const t = ms(ts);
  if (!t) return '';
  const d = new Date(t);
  return [d.getHours(), d.getMinutes(), d.getSeconds()].map((n) => String(n).padStart(2, '0')).join(':');
}

/** "1.2 s" / "340 ms" / "2 min". */
export function durationText(msValue) {
  const v = Number(msValue);
  if (!Number.isFinite(v) || v < 0) return '';
  if (v < 1000) return `${Math.round(v)} ms`;
  if (v < 60000) return `${(v / 1000).toFixed(1)} s`;
  return `${Math.round(v / 60000)} min`;
}
