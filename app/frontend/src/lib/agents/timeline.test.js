import { describe, expect, test } from 'vitest';
import { applyFilters, buildItems, categoryOf, clockText, durationText, mergeBatch, summarize, confidenceLabel, nestSubagents } from './timeline.js';

const pre = { seq: 1, ts: '2026-10-11T10:42:01Z', source: 'hook', type: 'tool_pre', session_id: 's', tool_use_id: 't1', tool_name: 'Bash', attrs: { tool_input: { command: 'python analyze.py' } } };
const proc = { seq: 2, ts: '2026-10-11T10:42:02Z', source: 'proc', type: 'process_start', session_id: 's', tool_use_id: 't1', pid: 1234, confidence: 'strong', attrs: { cmdline: 'python analyze.py' } };
const conn = { seq: 3, ts: '2026-10-11T10:42:04Z', source: 'net', type: 'conn_open', session_id: 's', tool_use_id: 't1', pid: 1234, confidence: 'strong', anomaly: 'unattributed_connection', attrs: { proto: 'tcp', remote: '203.0.113.9:443', state: 'ESTABLISHED' } };
const post = { seq: 4, ts: '2026-10-11T10:42:05Z', source: 'hook', type: 'tool_post', session_id: 's', tool_use_id: 't1', tool_name: 'Bash', attrs: { response: { bytes: 120, is_error: true } } };
const api = { seq: 5, ts: '2026-10-11T10:42:06Z', source: 'otel', type: 'api_request', session_id: 's', attrs: { model: 'claude-x', duration_ms: 1500, cost_usd: 0.0123 } };
const start = { seq: 0, ts: '2026-10-11T10:41:00Z', source: 'hook', type: 'session_start', session_id: 's', attrs: { start_source: 'startup', cwd: '/p' } };

describe('timeline grouping', () => {
  test('tool calls become one item with children, status and duration', () => {
    const items = buildItems([start, pre, proc, conn, post, api]);
    expect(items.map((i) => i.kind)).toEqual(['event', 'tool', 'event']);
    const tool = items[1];
    expect(tool.tool).toBe('Bash');
    expect(tool.pre).toBe(pre);
    expect(tool.post).toBe(post);
    expect(tool.processes).toEqual([proc]);
    expect(tool.connections).toEqual([conn]);
    expect(tool.anomalies).toEqual(['unattributed_connection']);
    expect(tool.status).toBe('failed');
    expect(tool.durationMs).toBe(4000);
    expect(items[0].events[0]).toBe(api);
    expect(items[2].events[0]).toBe(start);
  });

  test('a tool without a post event is running; otel duration fills in', () => {
    const [running] = buildItems([pre]);
    expect(running.status).toBe('running');
    expect(running.durationMs).toBeUndefined();
    const result = { seq: 2, ts: '2026-10-11T10:42:03Z', source: 'otel', type: 'tool_result', tool_use_id: 't1', attrs: { success: 'true', duration_ms: 900 } };
    const [done] = buildItems([{ ...pre, ts: '' }, result]);
    expect(done.status).toBe('ok');
    expect(done.durationMs).toBe(900);
  });

  test('filters hide categories; tool groups count as tools', () => {
    const items = buildItems([start, pre, proc, api]);
    expect(applyFilters(items, { tools: false }).map((i) => i.kind)).toEqual(['event', 'event']);
    expect(applyFilters(items, { api: false, session: false }).map((i) => i.kind)).toEqual(['tool']);
    expect(categoryOf(proc)).toBe('processes');
    expect(categoryOf(conn)).toBe('network');
    expect(categoryOf(api)).toBe('api');
    expect(categoryOf(start)).toBe('session');
  });
});

describe('timeline text', () => {
  test('summaries per type', () => {
    expect(summarize(pre)).toBe('python analyze.py');
    expect(summarize(proc)).toBe('python analyze.py');
    expect(summarize(conn)).toBe('tcp · 203.0.113.9:443 · ESTABLISHED');
    expect(summarize(api)).toBe('claude-x · 1500 ms · $0.0123');
    expect(summarize(start)).toBe('startup · /p');
    expect(summarize({ type: 'tool_pre', tool_name: 'Read', attrs: { tool_input: { file_path: '/p/a.go' } } })).toBe('/p/a.go');
    expect(summarize({ type: 'tool_pre', tool_name: 'Grep', attrs: { tool_input: { pattern: 'foo', path: 'src' } } })).toBe('foo in src');
    expect(confidenceLabel(proc)).toBe('●●●');
    expect(confidenceLabel({ confidence: 'none' })).toBe('○○○');
    expect(confidenceLabel(pre)).toBe('');
  });

  test('mergeBatch dedupes by seq and keeps order', () => {
    const merged = mergeBatch([post, pre], [proc, { ...post, attrs: { response: { bytes: 1 } } }]);
    expect(merged.map((e) => e.seq)).toEqual([1, 2, 4]);
    expect(merged[2].attrs.response.bytes).toBe(1);
  });

  test('clock and duration text', () => {
    expect(clockText('nope')).toBe('');
    expect(clockText('2026-10-11T10:42:01Z')).toMatch(/^\d{2}:\d{2}:01$/);
    expect(durationText(340)).toBe('340 ms');
    expect(durationText(1234)).toBe('1.2 s');
    expect(durationText(125000)).toBe('2 min');
    expect(durationText(-1)).toBe('');
  });

  test('subagent tool calls nest under the parent Task call via span ids', () => {
    const task = { seq: 1, ts: '2026-10-11T10:50:00Z', source: 'hook', type: 'tool_pre', tool_use_id: 'task1', tool_name: 'Task', attrs: { tool_input: { description: 'explore' } } };
    const taskSpan = { seq: 2, ts: '2026-10-11T10:50:00Z', source: 'otel', type: 'span_tool', tool_use_id: 'task1', tool_name: 'Task', attrs: { span_id: 'S1', trace_id: 'T' } };
    const subPre = { seq: 3, ts: '2026-10-11T10:50:02Z', source: 'hook', type: 'tool_pre', tool_use_id: 'sub1', tool_name: 'Bash', attrs: { tool_input: { command: 'grep -r foo' } } };
    const subSpan = { seq: 4, ts: '2026-10-11T10:50:02Z', source: 'otel', type: 'span_tool', tool_use_id: 'sub1', tool_name: 'Bash', attrs: { span_id: 'S2', parent_span_id: 'S1', trace_id: 'T' } };
    const other = { seq: 5, ts: '2026-10-11T10:51:00Z', source: 'hook', type: 'tool_pre', tool_use_id: 'x', tool_name: 'Read', attrs: { tool_input: { file_path: '/a' } } };
    const items = buildItems([task, taskSpan, subPre, subSpan, other]);
    expect(items.map((i) => i.toolUseId)).toEqual(['x', 'task1']);
    expect(items[1].children.map((c) => c.toolUseId)).toEqual(['sub1']);
    expect(items[1].children[0].tool).toBe('Bash');
    // Without spans nothing nests.
    expect(nestSubagents(buildItems([task, subPre])).length).toBe(2);
  });
});
