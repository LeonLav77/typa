/**
 * Reads the analytics endpoint and turns it into something legible.
 *
 * This is the ONLY place in the codebase that decides what data means. The
 * thresholds below are judgement calls, which is exactly why they live here
 * and not in capture or storage: changing one reinterprets all history.
 *
 * The central rule: a single run can only support observations about itself
 * ("you mistyped m three times in this test"). Claims about the typist ("m is
 * your weakest key") require repetition across runs, because one test cannot
 * distinguish a habit from a bad minute. Everything below enforces that split.
 */

export async function load(typist, runID) {
  const params = new URLSearchParams();
  if (typist) params.set('typist', typist);
  if (runID) params.set('run', runID);
  const qs = params.toString();
  const res = await fetch(`/api/insights${qs ? `?${qs}` : ''}`, {
    headers: { accept: 'application/json' },
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

/* ---------------------------------------------------------------- findings */

/**
 * Evidence gates. A claim about the typist is not made until there is enough
 * data to tell a habit from a coincidence.
 *
 * These are deliberately strict. The failure they prevent -- announcing "word
 * 22 is where you pause most often" on the strength of one pause in one run --
 * is worse than saying nothing, because it teaches the user to distrust
 * everything else on the screen.
 */
const GATE = {
  runs: 5,          // minimum runs before ANY lifetime claim is made
  keys: 300,        // minimum keystrokes behind a lifetime claim
  attempts: 25,     // per-key attempts before calling a key weak
  confusions: 4,    // times a specific mix-up must recur
  recovery: 60,     // keys measured after errors
  doubles: 40,      // double-letter instances
  bucketKeys: 40,   // keys in a time bucket before trusting its mean
  hesitations: 4,   // pauses at one word position before calling it a pattern
};

const SLOWER = 1.2; // 20% slower than baseline is worth mentioning
const FASTER = 0.9;

/**
 * What happened in the run just finished. These are observations, not verdicts:
 * every statement is scoped to "this run" in its wording, because that is all
 * one test can support.
 */
export function runNotes(d) {
  const out = [];
  const r = d?.this;
  if (!r) return out;

  // Mistakes actually made, named specifically. Useful immediately and makes
  // no claim about tendency.
  const misses = (r.misses || []).filter((m) => m.expected && m.typed);
  if (misses.length) {
    const top = misses.slice(0, 3)
      .map((m) => `${label(m.typed)} for ${label(m.expected)}${m.times > 1 ? ` (\u00d7${m.times})` : ''}`)
      .join(', ');
    out.push({
      kind: 'run-misses',
      tone: 'note',
      text: `Missed this run: ${top}.`,
      detail: `${r.errors} slip${r.errors === 1 ? '' : 's'} in ${r.keystrokes} keystrokes.`,
    });
  } else if (r.keystrokes) {
    out.push({
      kind: 'run-clean',
      tone: 'good',
      text: `Clean run — no mistyped keys.`,
      detail: `${r.keystrokes} keystrokes.`,
    });
  }

  // Slowest keys in this run. Framed as "here", not "always".
  const slow = (r.slowKeys || []).filter((k) => k.meanFlight > 0).slice(0, 3);
  if (slow.length >= 3 && r.cleanFlight) {
    const worst = slow.filter((k) => k.meanFlight > r.cleanFlight * SLOWER);
    if (worst.length) {
      out.push({
        kind: 'run-slow',
        tone: 'note',
        text: `Slowest keys here: ${worst.map((k) => label(k.key)).join(', ')}.`,
        detail: worst.map((k) => `${label(k.key)} ${k.meanFlight}ms`).join(' · ')
          + ` vs ${r.cleanFlight}ms typical.`,
      });
    }
  }

  // Where this run's longest pause fell -- a fact about this text, not a habit.
  const pauses = (r.byWord || []).filter((w) => w.hesitations > 0);
  if (pauses.length) {
    const worst = pauses.reduce((a, b) => (b.meanFlight > a.meanFlight ? b : a));
    out.push({
      kind: 'run-pause',
      tone: 'note',
      text: `Longest pause came at word ${worst.word + 1}.`,
      detail: `${worst.meanFlight}ms per key there.`,
    });
  }
  return out;
}

/**
 * What you are like as a typist. Returns [] until there is enough evidence,
 * and says so via `gate()` rather than silently showing nothing.
 */
export function findings(d) {
  const out = [];
  const a = d?.all;
  if (!a || a.runs < GATE.runs || a.keystrokes < GATE.keys) return out;

  out.push(...afterMistake(a));
  out.push(...focusDrift(a));
  out.push(...weakKeys(a));
  out.push(...confusionPairs(a));
  out.push(...doubleLetters(a));
  return out;
}

/**
 * How far off the evidence gate we are, so the UI can say "3 more runs" rather
 * than showing an unexplained empty panel.
 */
export function gate(d) {
  const a = d?.all;
  if (!a) return { ready: false, runs: 0, need: GATE.runs };
  return {
    ready: a.runs >= GATE.runs && a.keystrokes >= GATE.keys,
    runs: a.runs,
    need: GATE.runs,
    remaining: Math.max(0, GATE.runs - a.runs),
  };
}

/** "You slow down after a mistake" — the headline one. */
function afterMistake(a) {
  const base = a.cleanFlight;
  if (!base || !a.recovery?.length) return [];

  const after = a.recovery.filter((r) => r.since >= 1 && r.since <= 3);
  const keys = after.reduce((n, r) => n + r.keys, 0);
  if (keys < GATE.recovery) return [];

  const mean = after.reduce((n, r) => n + r.meanFlight * r.keys, 0) / keys;
  const ratio = mean / base;
  if (ratio < SLOWER) return [];

  const pct = Math.round((ratio - 1) * 100);
  return [{
    kind: 'recovery',
    tone: 'watch',
    text: `You slow down after a mistake — the next few keys take ${pct}% longer than usual.`,
    detail: `${Math.round(mean)}ms per key after an error vs ${base}ms when clean, over ${keys} keys.`,
  }];
}

/**
 * Where attention drops. Compares against the run's OWN median bucket rather
 * than its first: the opening bucket is fastest by construction (no fatigue
 * yet), so using it as the baseline overstates every later slowdown.
 */
function focusDrift(a) {
  const out = [];

  const pace = (a.pace || []).filter((p) => p.keys >= GATE.bucketKeys);
  if (pace.length >= 4) {
    const flights = pace.map((p) => p.meanFlight).sort((x, y) => x - y);
    const median = flights[Math.floor(flights.length / 2)] || 1;
    const worst = pace.reduce((x, y) => (y.meanFlight > x.meanFlight ? y : x));
    if (worst.meanFlight > median * SLOWER && worst.second > 0) {
      out.push({
        kind: 'focus-time',
        tone: 'watch',
        text: `You tend to drift around ${worst.second}s in.`,
        detail: `${worst.meanFlight}ms per key there vs ${median}ms typical.`,
      });
    }
  }

  const words = (a.byWord || []).filter((w) => w.hesitations >= GATE.hesitations);
  if (words.length) {
    const worst = words.reduce((x, y) => (y.hesitations > x.hesitations ? y : x));
    out.push({
      kind: 'focus-word',
      tone: 'watch',
      text: `Word ${worst.word + 1} is where you pause most often.`,
      detail: `${worst.hesitations} pauses over 500ms recorded there, across runs.`,
    });
  }
  return out;
}

/** Keys that are reliably worse than the rest. */
function weakKeys(a) {
  const keys = (a.keys || []).filter((k) => k.attempts >= GATE.attempts);
  if (keys.length < 8) return [];

  const weak = keys.filter((k) => k.accuracy < 95).slice(0, 5);
  if (!weak.length) return [];

  const list = weak.map((k) => `${label(k.key)} (${k.accuracy}%)`).join(', ');
  return [{
    kind: 'weak-keys',
    tone: 'watch',
    text: `Least reliable keys: ${list}.`,
    detail: `Measured over ${weak.reduce((n, k) => n + k.attempts, 0)} attempts.`,
  }];
}

/** Specific key-for-key mix-ups that keep recurring. */
function confusionPairs(a) {
  const pairs = (a.confusions || []).filter((c) => c.times >= GATE.confusions).slice(0, 3);
  if (!pairs.length) return [];
  return pairs.map((c) => ({
    kind: `confusion-${c.expected}-${c.typed}`,
    tone: 'watch',
    text: `You hit ${label(c.typed)} when you meant ${label(c.expected)}.`,
    detail: `${c.times} times${c.code ? ` — physical key ${c.code}` : ''}.`,
  }));
}

/** Repeated letters, compared against everything else. */
function doubleLetters(a) {
  const s = a.doubles;
  if (!s || s.doubles < GATE.doubles || !s.singleFlight) return [];

  const ratio = s.doubleFlight / s.singleFlight;
  if (ratio <= FASTER) {
    return [{
      kind: 'doubles',
      tone: 'good',
      text: `You are good at double letters — faster than your usual pace.`,
      detail: `${s.doubleFlight}ms vs ${s.singleFlight}ms per key, over ${s.doubles} of them.`,
    }];
  }
  if (ratio >= SLOWER) {
    return [{
      kind: 'doubles',
      tone: 'watch',
      text: `Double letters slow you down.`,
      detail: `${s.doubleFlight}ms vs ${s.singleFlight}ms per key, over ${s.doubles} of them.`,
    }];
  }
  return [];
}

function label(k) {
  if (k === ' ') return 'space';
  return k || '?';
}

/* ------------------------------------------------------------------ charts */

/**
 * A minimal line chart as inline SVG. No chart library: the shapes here are
 * simple, and 50KB of dependency to draw a polyline is a bad trade.
 *
 * Returns an SVG string. `points` is [{x, y}], already in data units.
 */
export function lineChart(points, opts = {}) {
  const w = opts.width ?? 560;
  const h = opts.height ?? 130;
  const pad = { top: 8, right: 8, bottom: 18, left: 28 };

  if (!points.length) return '';

  const xs = points.map((p) => p.x);
  const ys = points.map((p) => p.y);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  // Always include 0 on the y axis so bar heights are honest.
  const minY = 0, maxY = Math.max(...ys, 1);

  const plotW = w - pad.left - pad.right;
  const plotH = h - pad.top - pad.bottom;
  const sx = (x) => pad.left + (maxX === minX ? plotW / 2 : ((x - minX) / (maxX - minX)) * plotW);
  const sy = (y) => pad.top + plotH - ((y - minY) / (maxY - minY)) * plotH;

  const line = points.map((p, i) => `${i ? 'L' : 'M'}${sx(p.x).toFixed(1)},${sy(p.y).toFixed(1)}`).join('');
  const area = `${line}L${sx(maxX).toFixed(1)},${sy(0).toFixed(1)}L${sx(minX).toFixed(1)},${sy(0).toFixed(1)}Z`;

  // Two reference labels only: the maximum and zero. More would crowd it.
  const gridY = [maxY, maxY / 2].map(
    (v) => `<line class="grid" x1="${pad.left}" y1="${sy(v).toFixed(1)}" x2="${w - pad.right}" y2="${sy(v).toFixed(1)}"/>`
       + `<text class="tick" x="${pad.left - 6}" y="${(sy(v) + 3).toFixed(1)}" text-anchor="end">${Math.round(v)}</text>`,
  ).join('');

  const dots = points.length <= 40
    ? points.map((p) => `<circle class="dot" cx="${sx(p.x).toFixed(1)}" cy="${sy(p.y).toFixed(1)}" r="2"/>`).join('')
    : '';

  return `<svg class="chart" viewBox="0 0 ${w} ${h}" preserveAspectRatio="none" role="img" aria-label="${opts.label ?? 'chart'}">`
    + gridY
    + `<path class="area" d="${area}"/><path class="line" d="${line}"/>${dots}`
    + `</svg>`;
}

/** Horizontal bars, used for the after-a-mistake comparison. */
export function barRow(items, opts = {}) {
  if (!items.length) return '';
  const max = Math.max(...items.map((i) => i.value), 1);
  return `<div class="bars">` + items.map((i) => {
    const pct = (i.value / max) * 100;
    return `<div class="bar-row">`
      + `<span class="bar-label">${i.label}</span>`
      + `<span class="bar-track"><span class="bar-fill ${i.tone ?? ''}" style="width:${pct.toFixed(1)}%"></span></span>`
      + `<span class="bar-value">${i.display ?? i.value}${opts.unit ?? ''}</span>`
      + `</div>`;
  }).join('') + `</div>`;
}
