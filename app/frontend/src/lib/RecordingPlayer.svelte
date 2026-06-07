<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  import { t } from './i18n.js';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';

  const dispatch = createEventDispatcher();

  export let recordings = [];
  export let aggAvailable = false;
  export let loadRecording = async (id) => '';

  let selectedId = null;
  let castData = null;
  let header = null;
  let frames = [];

  // Playback state
  let playing = false;
  let playbackSpeed = 1;
  let currentTime = 0;
  let totalDuration = 0;
  let frameIndex = 0;

  // Timing: wallclock-based playback
  let playStartWall = 0;    // performance.now() when play started
  let playStartCast = 0;    // cast-time when play started
  let rafId = null;

  // xterm
  let termEl;
  let term = null;
  let fitAddon = null;

  // Export state
  let exporting = false;
  let exportingGif = false;

  // Trim state
  let cutRegions = [];
  let trimMode = false;
  let cutStart = null;

  onMount(() => {
    term = new Terminal({
      cursorBlink: false,
      disableStdin: true,
      fontSize: 13,
      fontFamily: "'JetBrains Mono', 'Cascadia Code', 'Fira Code', Menlo, monospace",
      theme: {
        background: '#1a1b26',
        foreground: '#a9b1d6',
        cursor: '#c0caf5',
        selectionBackground: '#33467c',
        black: '#15161e', red: '#f7768e', green: '#9ece6a', yellow: '#e0af68',
        blue: '#7aa2f7', magenta: '#bb9af7', cyan: '#7dcfff', white: '#a9b1d6',
        brightBlack: '#414868', brightRed: '#f7768e', brightGreen: '#9ece6a',
        brightYellow: '#e0af68', brightBlue: '#7aa2f7', brightMagenta: '#bb9af7',
        brightCyan: '#7dcfff', brightWhite: '#c0caf5',
      },
    });
    fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
  });

  onDestroy(() => {
    stopPlayback();
    if (term) term.dispose();
  });

  function parseCast(data) {
    const lines = data.trim().split('\n');
    if (lines.length === 0) return { header: null, frames: [] };

    const hdr = JSON.parse(lines[0]);
    const frms = [];
    for (let i = 1; i < lines.length; i++) {
      try {
        const f = JSON.parse(lines[i]);
        frms.push({ time: f[0], type: f[1], data: f[2] });
      } catch (e) { /* skip malformed */ }
    }
    return { header: hdr, frames: frms };
  }

  async function selectRecording(rec) {
    stopPlayback();
    selectedId = rec.id;
    dispatch('select', rec.id);

    try {
      castData = await loadRecording(rec.id);
    } catch (e) {
      return;
    }

    if (!castData) return;
    const parsed = parseCast(castData);
    header = parsed.header;
    frames = parsed.frames;
    totalDuration = frames.length > 0 ? frames[frames.length - 1].time : 0;
    currentTime = 0;
    frameIndex = 0;
    cutRegions = [];
    cutStart = null;
    trimMode = false;

    await tick();
    if (termEl && term) {
      if (!termEl.querySelector('.xterm')) {
        term.open(termEl);
      }
      if (header) {
        term.resize(header.width || 80, header.height || 24);
      }
      fitAddon.fit();
      term.reset();
    }
  }

  function applyFrame(frame) {
    if (frame.type === 'o' && term) {
      term.write(frame.data);
    } else if (frame.type === 'r' && term) {
      const parts = frame.data.split('x');
      if (parts.length === 2) {
        const cols = parseInt(parts[0]);
        const rows = parseInt(parts[1]);
        if (cols > 0 && rows > 0) term.resize(cols, rows);
      }
    }
  }

  function play() {
    if (!frames.length) return;

    if (frameIndex >= frames.length) {
      // Restart from beginning
      frameIndex = 0;
      currentTime = 0;
      if (term) term.reset();
    }

    playStartWall = performance.now();
    playStartCast = currentTime;
    playing = true;
    rafId = requestAnimationFrame(tick_playback);
  }

  function isInCut(time) {
    for (const c of cutRegions) {
      if (time >= c.start && time < c.end) return c;
    }
    return null;
  }

  function tick_playback(now) {
    if (!playing) return;

    // Calculate current cast-time based on real elapsed wall time and speed
    const wallElapsed = (now - playStartWall) / 1000; // seconds
    const castElapsed = wallElapsed * playbackSpeed;
    let targetCastTime = playStartCast + castElapsed;

    // Skip over cut regions
    const cut = isInCut(targetCastTime);
    if (cut) {
      const skipped = cut.end - targetCastTime;
      targetCastTime = cut.end;
      // Adjust wall anchor to account for the skip
      playStartWall -= (skipped / playbackSpeed) * 1000;
    }

    // Apply all frames up to targetCastTime
    while (frameIndex < frames.length && frames[frameIndex].time <= targetCastTime) {
      // Skip frames inside cut regions
      if (!isInCut(frames[frameIndex].time)) {
        applyFrame(frames[frameIndex]);
      }
      currentTime = frames[frameIndex].time;
      frameIndex++;
    }

    // Update currentTime smoothly between frames (for progress bar)
    if (frameIndex < frames.length) {
      currentTime = Math.min(targetCastTime, totalDuration);
    }

    if (frameIndex >= frames.length) {
      currentTime = totalDuration;
      playing = false;
      rafId = null;
      return;
    }

    rafId = requestAnimationFrame(tick_playback);
  }

  function pause() {
    playing = false;
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = null;
    }
  }

  function stopPlayback() {
    pause();
    frameIndex = 0;
    currentTime = 0;
    if (term) term.reset();
  }

  function seekTo(percent) {
    const wasPlaying = playing;
    pause();
    if (!term) return;
    term.reset();

    const targetTime = (percent / 100) * totalDuration;
    frameIndex = 0;
    currentTime = 0;

    for (let i = 0; i < frames.length; i++) {
      if (frames[i].time > targetTime) break;
      applyFrame(frames[i]);
      currentTime = frames[i].time;
      frameIndex = i + 1;
    }

    // Snap to exact target for smooth slider
    currentTime = targetTime;

    if (wasPlaying) {
      play();
    }
  }

  function setSpeed(speed) {
    if (playing) {
      // Anchor at current position with new speed
      pause();
      playbackSpeed = speed;
      play();
    } else {
      playbackSpeed = speed;
    }
  }

  function formatDuration(seconds) {
    const m = Math.floor(seconds / 60);
    const s = Math.floor(seconds % 60);
    return `${m}:${s.toString().padStart(2, '0')}`;
  }

  function formatTimeAgo(timestamp) {
    const diff = Math.floor(Date.now() / 1000) - timestamp;
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  async function handleExportScrubbed() {
    if (!selectedId) return;
    exporting = true;
    try {
      dispatch('exportscrubbed', selectedId);
    } finally {
      exporting = false;
    }
  }

  async function handleExportGif() {
    if (!selectedId) return;
    exportingGif = true;
    try {
      dispatch('exportgif', selectedId);
    } finally {
      exportingGif = false;
    }
  }

  function toggleTrimMode() {
    trimMode = !trimMode;
    if (!trimMode) {
      cutStart = null;
    }
  }

  function markIn() {
    cutStart = currentTime;
  }

  function markOut() {
    if (cutStart === null || currentTime <= cutStart) return;
    cutRegions = [...cutRegions, { start: cutStart, end: currentTime }];
    cutStart = null;
  }

  function removeCut(index) {
    cutRegions = cutRegions.filter((_, i) => i !== index);
  }

  function clearCuts() {
    cutRegions = [];
    cutStart = null;
  }

  function handleExportTrimmed() {
    if (!selectedId || cutRegions.length === 0) return;
    exporting = true;
    try {
      dispatch('exporttrimmed', { id: selectedId, cuts: cutRegions });
    } finally {
      exporting = false;
    }
  }

  function handleExportTrimmedGif() {
    if (!selectedId || cutRegions.length === 0) return;
    exportingGif = true;
    try {
      dispatch('exporttrimmedgif', { id: selectedId, cuts: cutRegions });
    } finally {
      exportingGif = false;
    }
  }

  function handleDelete(e, id) {
    e.stopPropagation();
    dispatch('delete', id);
  }

  $: progress = totalDuration > 0 ? (currentTime / totalDuration) * 100 : 0;
</script>

<div class="recordings-container">
  <div class="recordings-list">
    <div class="list-header">
      <h3>{$t('recordingPlayer.title')}</h3>
      <span class="recording-count">{recordings.length}</span>
    </div>

    {#if recordings.length === 0}
      <div class="empty-state">
        <span class="empty-icon">&#x23FA;</span>
        <p>{$t('recordingPlayer.empty')}</p>
        <p class="empty-hint">{$t('recordingPlayer.emptyHint')}</p>
      </div>
    {:else}
      <div class="list-items">
        {#each recordings as rec}
          <div
            class="recording-item"
            class:selected={selectedId === rec.id}
            on:click={() => selectRecording(rec)}
            on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') selectRecording(rec); }}
            role="button"
            tabindex="0"
          >
            <div class="rec-title">{rec.title || $t('recordingPlayer.untitled')}</div>
            <div class="rec-meta">
              <span class="rec-time">{formatTimeAgo(rec.timestamp)}</span>
              <span class="rec-sep">&middot;</span>
              <span class="rec-duration">{formatDuration(rec.duration)}</span>
              <span class="rec-sep">&middot;</span>
              <span class="rec-size">{formatSize(rec.size)}</span>
            </div>
            {#if rec.meta}
              <div class="rec-meta-extra">
                {#if rec.meta.sshHost}
                  <span class="rec-badge ssh">SSH: {rec.meta.sshHost}</span>
                {:else if rec.meta.sessionType}
                  <span class="rec-badge">{rec.meta.sessionType}</span>
                {/if}
              </div>
            {/if}
            <button class="rec-delete" on:click={(e) => handleDelete(e, rec.id)} title={$t('recordingPlayer.deleteRecording')}>&#x2715;</button>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="player-area">
    {#if selectedId}
      <div class="player-terminal" bind:this={termEl}></div>

      <div class="player-controls">
        <div class="control-row">
          <button class="ctrl-btn" on:click={stopPlayback} title={$t('recordingPlayer.stop')}>&#x23F9;</button>
          {#if playing}
            <button class="ctrl-btn" on:click={pause} title={$t('recordingPlayer.pause')}>&#x23F8;</button>
          {:else}
            <button class="ctrl-btn play-btn" on:click={play} title={$t('recordingPlayer.play')}>&#x25B6;</button>
          {/if}

          <div class="speed-group">
            <button class="speed-btn" class:active={playbackSpeed === 1} on:click={() => setSpeed(1)}>1x</button>
            <button class="speed-btn" class:active={playbackSpeed === 2} on:click={() => setSpeed(2)}>2x</button>
            <button class="speed-btn" class:active={playbackSpeed === 4} on:click={() => setSpeed(4)}>4x</button>
          </div>

          <button class="ctrl-btn trim-btn" class:active={trimMode} on:click={toggleTrimMode} title={$t('recordingPlayer.trimMode')}>&#x2702;</button>

          {#if trimMode}
            <button class="ctrl-btn mark-btn" on:click={markIn} title={$t('recordingPlayer.markIn')} class:active={cutStart !== null}>I</button>
            <button class="ctrl-btn mark-btn" on:click={markOut} title={$t('recordingPlayer.markOut')} disabled={cutStart === null}>O</button>
          {/if}

          <span class="time-display">{formatDuration(currentTime)} / {formatDuration(totalDuration)}</span>
        </div>

        <div class="seek-row">
          <div class="seek-track">
            <input
              type="range"
              class="seek-slider"
              min="0"
              max="100"
              step="0.1"
              value={progress}
              on:input={(e) => seekTo(parseFloat(e.target.value))}
            />
            <div class="cut-overlay">
              {#each cutRegions as cut}
                <div
                  class="cut-region"
                  style="left: {(cut.start / totalDuration) * 100}%; width: {((cut.end - cut.start) / totalDuration) * 100}%"
                ></div>
              {/each}
              {#if cutStart !== null}
                <div
                  class="cut-region in-progress"
                  style="left: {(cutStart / totalDuration) * 100}%; width: {((currentTime - cutStart) / totalDuration) * 100}%"
                ></div>
              {/if}
            </div>
          </div>
        </div>

        {#if cutRegions.length > 0}
          <div class="cut-list">
            {#each cutRegions as cut, i}
              <div class="cut-item">
                <span class="cut-label">{$t('recordingPlayer.cutLabel', { n: i + 1 })}</span>
                <span class="cut-times">{formatDuration(cut.start)} – {formatDuration(cut.end)}</span>
                <span class="cut-duration">({formatDuration(cut.end - cut.start)})</span>
                <button class="cut-delete" on:click={() => removeCut(i)} title={$t('recordingPlayer.removeCut')}>&#x2715;</button>
              </div>
            {/each}
          </div>
        {/if}

        <div class="export-row">
          <button class="export-btn" on:click={handleExportScrubbed} disabled={exporting}>
            {exporting ? $t('recordingPlayer.exporting') : $t('recordingPlayer.exportScrubbed')}
          </button>
          {#if aggAvailable}
            {#if cutRegions.length > 0}
              <button class="export-btn trimmed" on:click={handleExportTrimmedGif} disabled={exportingGif}>
                {exportingGif ? $t('recordingPlayer.generating') : $t('recordingPlayer.exportGifTrimmed')}
              </button>
            {:else}
              <button class="export-btn" on:click={handleExportGif} disabled={exportingGif}>
                {exportingGif ? $t('recordingPlayer.generating') : $t('recordingPlayer.exportGif')}
              </button>
            {/if}
          {/if}
          {#if cutRegions.length > 0}
            <button class="export-btn clear-btn" on:click={clearCuts}>{$t('recordingPlayer.clearCuts')}</button>
          {/if}
        </div>
      </div>
    {:else}
      <div class="no-selection">
        <span class="no-sel-icon">&#x25B6;</span>
        <p>{$t('recordingPlayer.selectRecording')}</p>
      </div>
    {/if}
  </div>
</div>
