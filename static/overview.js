/**
 * The interpreted view: what the data says about you, in sentences.
 *
 * Reads the same endpoint the results screen uses, but renders the lifetime
 * half — the part that needs many runs before it means anything. The results
 * screen deliberately shows none of this, so that finishing a test reports on
 * that test rather than reopening a dashboard.
 */
import { load, findings, gate, lineChart, barRow } from './insights.js';
import { install } from './menu.js';

install();

const root = document.querySelector('[data-overview]');

load(localStorage.getItem('typing:typist') || null)
  .then(render)
  .catch((err) => {
    console.error(err);
    root.innerHTML = `<p class="pending">Could not read your data.</p>`;
  });

function render(d) {
  const g = gate(d);
  const found = findings(d);
  const parts = [];

  if (!g.ready) {
    parts.push(`
      <p class="pending">
        Not enough data yet — patterns need <b>${g.remaining}</b>
        more ${g.remaining === 1 ? 'run' : 'runs'}.
        <span class="f-detail">One test can show what happened, not what you tend to do.</span>
      </p>`);
  } else if (!found.length) {
    parts.push(`
      <p class="pending">
        Nothing stands out yet.
        <span class="f-detail">No pattern in your data currently clears the threshold worth reporting.</span>
      </p>`);
  } else {
    parts.push(`<ul class="findings">` + found.map((f) => `
      <li class="${f.tone}">
        <span class="f-text">${escape(f.text)}</span>
        <span class="f-detail">${escape(f.detail)}</span>
      </li>`).join('') + `</ul>`);
  }

  const charts = buildCharts(d);
  if (charts.length) {
    parts.push(`<div class="panels">` + charts.join('') + `</div>`);
  }

  root.innerHTML = parts.join('');
}

function buildCharts(d) {
  const all = d.all;
  const out = [];

  const runs = [...(all?.recent || [])].reverse();
  if (runs.length >= 3) {
    out.push(panel('recent runs', 'wpm',
      lineChart(runs.map((r, i) => ({ x: i, y: r.wpm })), { label: 'wpm across runs' })));
  }

  const pace = (all?.pace || []).filter((p) => p.keys >= 40);
  if (pace.length >= 4) {
    out.push(panel('pace through a test', 'ms per key',
      lineChart(pace.map((p) => ({ x: p.second, y: p.meanFlight })), { label: 'pace over time' })));
  }

  const rec = (all?.recovery || []).filter((r) => r.since >= 1 && r.since <= 4);
  const recKeys = rec.reduce((n, r) => n + r.keys, 0);
  if (rec.length && all?.cleanFlight && recKeys >= 60) {
    out.push(panel('after a mistake', 'ms per key', barRow([
      { label: 'normally', value: all.cleanFlight, display: all.cleanFlight, tone: 'good' },
      ...rec.map((r) => ({
        label: `+${r.since} key${r.since === 1 ? '' : 's'}`,
        value: r.meanFlight,
        display: r.meanFlight,
        tone: r.meanFlight > all.cleanFlight * 1.2 ? 'bad' : '',
      })),
    ], { unit: 'ms' })));
  }
  return out;
}

function panel(title, unit, body) {
  if (!body) return '';
  return `<figure class="panel">
    <figcaption>${title} <span>${unit}</span></figcaption>
    <div class="plot">${body}</div>
  </figure>`;
}

/** Findings are built from database values; escape before inserting as HTML. */
function escape(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));
}
