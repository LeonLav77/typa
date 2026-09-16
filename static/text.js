/**
 * What the test is made of.
 *
 * Two kinds of choice live here. The SOURCE is exclusive — English, one hand
 * only, or a programming vocabulary — because a test is drawn from one pool.
 * The flags below it compose: `numbers` adds tokens, `caps` and `punctuation`
 * decorate whatever the pool produced.
 *
 * Splitting them that way is what keeps "left hand" and "laravel" from being
 * special cases. A new language is one entry in SOURCES and nothing else
 * changes, where a flag per language would need a rule for what "go + python"
 * means — a question with no good answer.
 *
 * The server owns generation. This only records the choice; the flags travel
 * on the query string when a test is dealt.
 */
import {
  mode,
  toggleMode,
  setLevel,
  setSource,
  sourceById,
  modeQuery,
  modeTag,
  LEVEL_OF,
  LEVELS,
  SOURCES,
} from './context.js';
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

// Sources and toggles share one picker, so typing narrows across both and
// enter takes whatever it lands on. `kind` is what tells them apart when a
// choice is made: a source is set, a flag is flipped.
const items = () => [
  ...SOURCES.map((s) => ({
    kind: 'source',
    value: s.id,
    label: s.label,
    // No alias starting with a letter the toggles below already own: `w`
    // must keep meaning the words flag, not the English pool.
    alias: s.id === 'words' ? ['english'] : [s.id],
  })),
  // Symbols is in the picker rather than being a checkbox off to one side,
  // because this page is driven from the keyboard and a control that needs a
  // mouse is one that cannot be reached at all. It appears only while the
  // chosen source has a symbol flavour: a row that silently does nothing on
  // English would be worse than no row.
  ...(sourceById(mode().source).code
    ? [{ kind: 'text', value: 'symbols', label: 'symbols', alias: ['sigils'] }]
    : []),
  ...TEXT.map(([value, label, , alias]) => ({ kind: 'text', value, label, alias })),
];

const picker = installPicker({
  items,
  onChoose: (item) => {
    if (item.kind === 'source') setSource(item.value);
    else toggleMode(item.value);
    draw();
    refreshSample();
  },
  onChange: () => draw(),
  // The cursor starts on the pool already in force, not on the top row. The
  // sources are an exclusive pick, so parking the cursor elsewhere marks a
  // second row on a group where exactly one thing is true — the same reason
  // /setup does this.
  start: (c) => c.kind === 'source' && c.value === mode().source,
  // Arrows move the slider of whatever the query has narrowed to, so the
  // levels are reachable without a mouse like everything else here. Adjusting
  // something switched off would be a silent no-op, so it turns it on first.
  onAdjust: (item, delta) => {
    if (item.kind === 'source') return; // a pool has no amount to adjust
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
  <span class="dim">or type a source or flag by name</span>`;

function draw() {
  const m = mode();
  const st = picker.state();

  root.innerHTML = `
    ${queryLine(st, {
      hint: HINT,
      verb: (c) =>
        c.kind === 'source'
          ? 'draws from it'
          : mode()[c.value]
            ? 'turns it off'
            : 'turns it on',
    })}
    ${sourceGroup(m, st)}
    ${group(m, st)}
    ${preview()}
  `;

  for (const el of root.querySelectorAll('button[data-source]')) {
    el.addEventListener('click', () => {
      setSource(el.dataset.source);
      picker.clear();
      draw();
      refreshSample();
    });
  }

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
 * The pool. An exclusive pick, drawn like /setup's groups rather than the
 * toggles below: exactly one is in force, so it gets a single-selection mark
 * and no checkbox.
 *
 * The symbols row hangs off the chosen source instead of standing alone,
 * because it means nothing on English — there are no sigils to add to "the".
 * Showing it only where it applies beats a row that silently does nothing.
 */
function sourceGroup(m, { query, hits, active }) {
  const chosen = sourceById(m.source);
  const hit = (v) => hits.some((h) => h.kind === 'source' && h.value === v);

  return `
    <section class="setup-group">
      <h2>source</h2>
      <div class="choices">
        ${SOURCES.map((s) => {
          const on = s.id === chosen.id;
          const matched = query ? hit(s.id) : false;
          const faded = query && !matched ? ' faded' : '';
          const ready =
            active && active.kind === 'source' && active.value === s.id ? ' ready' : '';
          return `
            <button type="button" data-source="${s.id}"
                    class="choice${on ? ' on' : ''}${faded}${ready}"
                    ${on ? 'aria-current="true"' : ''}>
              <span class="c-label">${escape(s.label)}</span>
              <span class="c-blurb">${escape(s.blurb)}</span>
            </button>`;
        }).join('')}
      </div>
      ${
        chosen.code
          ? `<div class="choices symbols-row">
               ${(() => {
                 const on = !!m.symbols;
                 const matched = query ? hits.some((h) => h.value === 'symbols') : false;
                 const faded = query && !matched ? ' faded' : '';
                 const ready =
                   active && active.kind === 'text' && active.value === 'symbols'
                     ? ' ready'
                     : '';
                 return `
                   <button type="button" data-value="symbols"
                           class="choice toggle${on ? ' on' : ''}${faded}${ready}"
                           aria-pressed="${on}">
                     <span class="c-label"><span class="box">${on ? '×' : ''}</span>symbols</span>
                     <span class="c-blurb">$request-&gt;input( rather than request — the real
                     thing, and much harder</span>
                   </button>`;
               })()}
             </div>`
          : ''
      }
    </section>`;
}

/**
 * The toggles. Unlike the exclusive picks above several can be on at once, so
 * they get a box rather than a single-selection mark.
 */
function group(m, { query, hits, active }) {
  const hit = (v) => hits.some((h) => h.kind === 'text' && h.value === v);

  return `
    <section class="setup-group">
      <h2>and also</h2>
      <div class="choices">
        ${TEXT.map(([value, label, blurb]) => {
          const on = !!m[value];
          const matched = query ? hit(value) : false;
          // While filtering, non-matches recede rather than disappearing: the
          // list stays put so the eye does not re-find it on every keystroke.
          const faded = query && !matched ? ' faded' : '';
          // `ready` marks the row the arrows and enter will act on, whether it
          // was reached by typing or by walking with up/down.
          // Matched on kind too: the source group also has a row valued
          // 'words', and without this both light up as the cursor.
          const ready =
            active && active.kind === 'text' && active.value === value ? ' ready' : '';
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
              ${on ? slider(value, m, active && active.kind === 'text' && active.value === value) : ''}
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
