/**
 * The setup screen: where you are and what you type on.
 *
 * It is driven entirely from the keyboard — type a letter to narrow the
 * choices, enter to take the one that remains — because a screen that needs a
 * mouse does not belong in a keyboard-only tool. The matching and key handling
 * are shared with /text, which uses the same interaction.
 *
 * What the TEXT is made of lives on /text, not here: these two tags label a
 * run, while that page changes what you are about to type. Keeping them apart
 * is also what stops either page needing to scroll.
 *
 * Everything here writes to localStorage via context.js and nothing else —
 * the tags are read at the end of a run, not stored on the server until then,
 * so changing a setting never rewrites history. Past runs keep the tag they
 * were given.
 */
import {
  resolve,
  overrides,
  setOverride,
  watch,
  describe,
  pairMoonlander,
  moonlanderAttached,
  hasHID,
  WORK_PLACE,
} from './context.js';
import { install as installPicker, queryLine } from './picker.js';
import { install as installMenu } from './menu.js';

installMenu();

const root = document.querySelector('[data-setup]');
const current = document.querySelector('[data-current]');

// There is no "automatic" row: every run is always tagged, with `home` and
// `moonlander` as the standing defaults. Detection replaces a default when it
// actually finds something; choosing here replaces both and is final.
//
// Every option is reachable in one keystroke bar `other`, which needs `ot` to
// clear `moonlander`'s o. The aliases are spares in the words people reach for.
const LOCATIONS = [
  ['work', 'work', `where ${WORK_PLACE} is`, ['labin', 'office']],
  ['home', 'home', 'the default', []],
  ['other', 'other', 'somewhere that is neither', []],
];

const DEVICES = [
  ['moonlander', 'moonlander', 'the default', ['zsa']],
  ['normal', 'normal keyboard', 'a laptop or membrane keyboard', ['laptop']],
  ['guest', 'someone else', 'another person typing — kept out of your stats', ['guest']],
];

let hidState = { supported: hasHID(), attached: false };

const items = () => [
  ...LOCATIONS.map(([value, label, , alias]) => ({ kind: 'location', value, label, alias })),
  ...DEVICES.map(([value, label, , alias]) => ({ kind: 'device', value, label, alias })),
];

const picker = installPicker({
  items,
  onChoose: (item) => choose(item.kind, item.value),
  onChange: () => draw(),
});

const HINT = `
  <kbd>↑</kbd><kbd>↓</kbd> <span class="dim">move ·</span>
  <kbd>enter</kbd> <span class="dim">choose ·</span>
  <span class="dim">or type</span> <kbd>w</kbd> <kbd>h</kbd> <kbd>m</kbd> <kbd>n</kbd> <kbd>s</kbd>`;

/**
 * Apply one choice. These are exclusive picks, and re-choosing the row already
 * in force releases it back to detection — which is what the old "automatic"
 * row did, without spending a choice on it.
 */
function choose(kind, value) {
  const cur = overrides();
  setOverride(kind, cur[kind] === value ? '' : value);
  draw();
}

function draw() {
  const o = overrides();
  const tags = resolve();
  const st = picker.state();

  current.textContent = describe(tags);

  root.innerHTML = `
    ${queryLine(st, {
      hint: HINT,
      verb: (c) => (overrides()[c.kind] === c.value ? 'releases it to detection' : 'sets it'),
    })}
    ${group('location', 'location', LOCATIONS, o.location, tags, st)}
    ${group('device', 'keyboard', DEVICES, o.device, tags, st)}
    ${hidPanel()}
  `;

  for (const el of root.querySelectorAll('button[data-kind]')) {
    el.addEventListener('click', () => choose(el.dataset.kind, el.dataset.value));
  }

  root.querySelector('[data-pair]')?.addEventListener('click', async () => {
    try {
      await pairMoonlander();
      hidState.attached = await moonlanderAttached();
    } catch (err) {
      console.error('pairing failed:', err);
    }
    draw();
  });
}

/** One labelled set of choices. The active one is marked, not merely styled. */
function group(kind, title, options, chosen, tags, { query, hits, active }) {
  const src = kind === 'location' ? tags.locationSrc : tags.deviceSrc;
  const resolved = kind === 'location' ? tags.location : tags.device;
  const hit = (v) => hits.some((h) => h.kind === kind && h.value === v);

  // How the currently active row got that way. A `manual` tag is the one that
  // can go stale without any outward sign, so it is the one worth naming.
  const why = { manual: 'you chose this', geo: 'detected', hid: 'detected', default: 'default' };

  return `
    <section class="setup-group">
      <h2>${title}</h2>
      <div class="choices">
        ${options
          .map(([value, label, blurb]) => {
            // `chosen` is the manual override; `resolved` is what a run would
            // actually be tagged with right now. With no override the two
            // differ, and it is the resolved one the user needs to see marked.
            const on = chosen ? chosen === value : resolved === value;
            // While filtering, non-matches recede rather than disappearing:
            // the list stays in one place so the eye does not have to re-find
            // it on every keystroke.
            const matched = query ? hit(value) : false;
            const faded = query && !matched ? ' faded' : '';
            // Marks the row enter will take, whether reached by typing or by
            // walking with up/down.
            const ready =
              active && active.kind === kind && active.value === value ? ' ready' : '';
            // The active row says how it was decided, so a manual choice left
            // over from last week is visible rather than silently in force.
            const extra = on ? ` <span class="resolved">${why[src] || ''}</span>` : '';
            return `
              <button type="button" data-kind="${kind}" data-value="${value}"
                      class="choice${on ? ' on' : ''}${faded}${ready}"
                      ${on ? 'aria-current="true"' : ''}>
                <span class="c-label">${label}${extra}</span>
                <span class="c-blurb">${blurb}</span>
              </button>`;
          })
          .join('')}
      </div>
    </section>`;
}

/**
 * The keyboard-detection panel. Automatic device detection needs a one-time
 * grant before the browser will admit the keyboard exists, so this states
 * plainly which of the three states it is in rather than silently failing.
 */
function hidPanel() {
  if (!hidState.supported) {
    return `<p class="note dim">
      This browser has no WebHID, so the keyboard cannot be detected
      automatically — set it by hand above.
    </p>`;
  }
  if (hidState.attached) {
    return `<p class="note dim">
      Moonlander detected. Automatic keyboard tagging is working.
    </p>`;
  }
  return `<p class="note dim">
    No ZSA keyboard visible yet. Grant access once and it is remembered for
    every later visit.
    <button type="button" class="inline-btn" data-pair>connect moonlander</button>
    <span class="dim">If the chooser is empty on Linux, the udev rule for
    vendor 3297 is missing.</span>
  </p>`;
}

watch(() => {
  moonlanderAttached().then((a) => {
    hidState.attached = a;
    draw();
  });
});

moonlanderAttached().then((a) => {
  hidState.attached = a;
  draw();
});

draw();
