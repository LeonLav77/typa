/**
 * What the test is made of.
 *
 * `words` and `numbers` are sources and compose; `caps` and `punctuation`
 * decorate whatever those produce. Turning words off and numbers on gives a
 * digits-only test without a mode named "numbers only" — the combination falls
 * out rather than being enumerated, which is what stops the list needing a new
 * entry for every pairing someone wants next.
 *
 * The server owns generation. This only records the choice; the flags travel
 * on the query string when a test is dealt.
 */
import { mode, toggleMode, setLevel, modeQuery, modeTag, LEVEL_OF, LEVELS } from './context.js';
import { install as installPicker, queryLine, escape } from './picker.js';
import { install as installMenu } from './menu.js';

installMenu();

const root = document.querySelector('[data-text]');

// Every option is reachable in one keystroke here: w, n, p, c. The aliases are
// spares -- `digits` and `text` say the same thing in the words people reach
// for -- and cost nothing while no first letter collides.
const TEXT = [
  ['words', 'words', 'random common words', ['text']],
  ['numbers', 'numbers', 'digits mixed in — on their own if words is off', ['digits']],
  ['punctuation', 'punctuation', 'commas, full stops, brackets and quotes', []],
  ['caps', 'capitals', 'sentence capitals — needs words to capitalise', ['uppercase']],
];

const items = () =>
  TEXT.map(([value, label, , alias]) => ({ kind: 'text', value, label, alias }));

const picker = installPicker({
  items,
  onChoose: (item) => {
    toggleMode(item.value);
    draw();
    refreshSample();
  },
  onChange: () => draw(),
  // Arrows move the slider of whatever the query has narrowed to, so the
  // levels are reachable without a mouse like everything else here. Adjusting
  // something switched off would be a silent no-op, so it turns it on first.
  onAdjust: (item, delta) => {
    const lvKey = LEVEL_OF[item.value];
    if (!lvKey) return;
    const m = mode();
    if (!m[item.value]) toggleMode(item.value);
    setLevel(lvKey, (mode()[lvKey] || 3) + delta);
    draw();
    refreshSample();
  },
});

const HINT = `
  <kbd>↑</kbd><kbd>↓</kbd> <span class="dim">move ·</span>
  <kbd>space</kbd> <span class="dim">toggle ·</span>
  <kbd>←</kbd><kbd>→</kbd> <span class="dim">how much ·</span>
  <span class="dim">or type</span> <kbd>w</kbd> <kbd>n</kbd> <kbd>p</kbd> <kbd>c</kbd>`;

function draw() {
  const m = mode();
  const st = picker.state();

  root.innerHTML = `
    ${queryLine(st, {
      hint: HINT,
      verb: (c) => (mode()[c.value] ? 'turns it off' : 'turns it on'),
    })}
    ${group(m, st)}
    ${preview()}
  `;

  for (const el of root.querySelectorAll('button[data-value]')) {
    el.addEventListener('click', () => {
      toggleMode(el.dataset.value);
      picker.clear();
      draw();
      refreshSample();
    });
  }

  for (const el of root.querySelectorAll('input[data-level]')) {
    // Deliberately NOT a full draw(): rebuilding the panel would replace the
    // very input being dragged, and the drag would die on the first stop.
    // Only the pips beside it change, so only they are updated.
    el.addEventListener('input', () => {
      const lv = Number(el.value);
      setLevel(el.dataset.level, lv);
      const pips = el.parentElement.querySelector('.s-value');
      if (pips) pips.textContent = '▪'.repeat(lv) + '·'.repeat(LEVELS - lv);
      // refreshSample() no-ops unless the tag actually changed, so dragging
      // across a stop and back costs one request rather than one per pixel.
      refreshSample();
    });
  }
}

/**
 * The toggles. Unlike the exclusive picks on /setup several can be on at once,
 * so they get a box rather than a single-selection mark.
 */
function group(m, { query, hits, only, active }) {
  const hit = (v) => hits.some((h) => h.value === v);

  return `
    <section class="setup-group">
      <div class="choices">
        ${TEXT.map(([value, label, blurb]) => {
          const on = !!m[value];
          const matched = query ? hit(value) : false;
          // While filtering, non-matches recede rather than disappearing: the
          // list stays put so the eye does not re-find it on every keystroke.
          const faded = query && !matched ? ' faded' : '';
          // `ready` marks the row the arrows and enter will act on, whether it
          // was reached by typing or by walking with up/down.
          const ready = active && active.value === value ? ' ready' : '';
          // Capitals need something to capitalise. With words off the flag is
          // still stored, it simply has nothing to act on — saying so beats
          // hiding the row and leaving the state unexplained.
          const moot =
            value === 'caps' && !m.words
              ? ` <span class="resolved">nothing to capitalise</span>`
              : '';
          return `
            <div class="choice-wrap">
              <button type="button" data-value="${value}"
                      class="choice toggle${on ? ' on' : ''}${faded}${ready}"
                      aria-pressed="${on}">
                <span class="c-label"><span class="box">${on ? '×' : ''}</span>${label}${moot}</span>
                <span class="c-blurb">${blurb}</span>
              </button>
              ${on ? slider(value, m, active && active.value === value) : ''}
            </div>`;
        }).join('')}
      </div>
    </section>`;
}

/**
 * How much of this one. Shown only while its flag is on, because a slider for
 * something switched off is a control with nothing to control.
 *
 * A range input rather than a number: the stops are coarse on purpose, and the
 * question is always "more or less of this", never "exactly what percent".
 */
function slider(key, m, isActive) {
  const lvKey = LEVEL_OF[key];
  if (!lvKey) return '';
  const lv = m[lvKey] || 3;
  return `
    <label class="slider${isActive ? ' active' : ''}">
      <span class="s-label">${isActive ? '<kbd>←</kbd><kbd>→</kbd>' : 'how much'}</span>
      <input type="range" min="1" max="${LEVELS}" step="1" value="${lv}"
             data-level="${lvKey}" aria-label="how much ${key}" />
      <span class="s-value">${'▪'.repeat(lv)}${'·'.repeat(LEVELS - lv)}</span>
    </label>`;
}

/**
 * A live sample of the current mode, fetched from the server.
 *
 * The server owns generation, so the only honest preview is one it produced —
 * reimplementing the generator here to avoid a request would be a second
 * source of truth that could disagree with the real thing.
 */
function preview() {
  // Carry the text already on screen through a redraw. draw() rebuilds the
  // whole panel, and filtering redraws on every keystroke -- without this the
  // sample would blink back to a placeholder as you type.
  const showing = root.querySelector('[data-sample]')?.textContent || '…';
  return `
    <section class="setup-group preview">
      <h2>sample</h2>
      <p class="sample" data-sample>${escape(showing)}</p>
    </section>`;
}

/**
 * Fetch a sample for the current mode.
 *
 * Only ever called when the mode actually changed, not on every keystroke:
 * narrowing a query redraws the list but asks for nothing, so typing stays
 * free. `seq` drops a response whose request has already been superseded.
 */
let pending = 0;
let showing = '';

async function refreshSample() {
  const tag = modeTag();
  if (tag === showing) return;
  showing = tag;

  const seq = ++pending;
  try {
    const res = await fetch(`/api/test?${modeQuery()}`, {
      headers: { accept: 'application/json' },
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const { words } = await res.json();
    if (seq !== pending) return; // a later toggle already asked for another
    const el = root.querySelector('[data-sample]');
    if (el) el.textContent = words.slice(0, 18).join(' ');
  } catch {
    if (seq !== pending) return;
    const el = root.querySelector('[data-sample]');
    if (el) el.textContent = 'could not load a sample';
    showing = ''; // let the next change retry
  }
}

draw();
refreshSample();
