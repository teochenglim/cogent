// Cogent UI — shared utilities

function formatDuration(ms) {
  if (ms == null) return '-';
  if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
  return Math.round(ms) + 'ms';
}

function formatBytes(n) {
  if (n == null || n === 0) return '-';
  if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB';
  if (n >= 1024)    return (n / 1024).toFixed(1) + ' KB';
  return n + ' B';
}

function formatCost(usd) {
  if (usd == null) return '-';
  if (usd >= 1) return '$' + usd.toFixed(2);
  return '$' + usd.toFixed(4);
}

function timeAgo(unixTs) {
  if (!unixTs) return '-';
  const diff = Date.now() / 1000 - unixTs;
  if (diff < 60)   return Math.round(diff) + 's ago';
  if (diff < 3600) return Math.round(diff / 60) + 'm ago';
  if (diff < 86400) return Math.round(diff / 3600) + 'h ago';
  return Math.round(diff / 86400) + 'd ago';
}

async function fetchJSON(url, opts = {}) {
  const res = await fetch(url, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`HTTP ${res.status}: ${text}`);
  }
  return res.json();
}

function scoreBadge(score) {
  if (score == null) return '<span class="text-muted">-</span>';
  const cls = score >= 0.8 ? 'score-good' : score >= 0.5 ? 'score-warn' : 'score-bad';
  return `<span class="score-badge ${cls}">${score.toFixed(2)}</span>`;
}

function truncate(s, n = 40) {
  if (!s) return '-';
  return s.length > n ? s.slice(0, n) + '…' : s;
}

async function lazyLoadPayload(spanId, field, targetEl, buttonEl) {
  buttonEl.disabled = true;
  buttonEl.innerHTML = '<span class="spinner"></span>';
  try {
    const data = await fetchJSON(`/api/spans/${spanId}/payload?field=${field}`);
    targetEl.textContent = data.content;
    buttonEl.textContent = 'Collapse';
    buttonEl.onclick = () => {
      targetEl.textContent = targetEl.dataset.preview || '';
      buttonEl.textContent = 'Load full';
      buttonEl.onclick = () => lazyLoadPayload(spanId, field, targetEl, buttonEl);
      buttonEl.disabled = false;
    };
  } catch (e) {
    targetEl.textContent = 'Error loading content: ' + e.message;
    buttonEl.textContent = 'Retry';
  }
  buttonEl.disabled = false;
}
