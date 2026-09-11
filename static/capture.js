/**
 * Raw capture. This file records what happened; it never decides what any of
 * it means. Interpretation ("you lose focus around 40s", "you confuse g and h")
 * happens later, in analysis, over the stored stream — so the rule here is to
 * write down everything cheap and reversible, and derive nothing.
 *
 * Two clocks are recorded once per run:
 *   wallStart  Date.now() at run start — locates the run in real time
 *              (time-of-day effects, streaks, "worse after 11pm").
 *   monoStart  performance.now() at run start — every event offset `t` is
 *              relative to this. Monotonic, so NTP jumps and clock drift
 *              cannot corrupt intervals.
 * Everything else is an integer millisecond offset from monoStart, which keeps
 * the payload small and makes every duration a subtraction.
 */

/** Event kinds. Strings, not enums, so old rows stay readable forever. */
export const KIND = {
  DOWN: 'down',     // keydown
  UP: 'up',         // keyup
  BLUR: 'blur',     // window lost focus
  FOCUS: 'focus',   // window regained focus
  HIDE: 'hide',     // tab hidden (visibilitychange)
  SHOW: 'show',     // tab visible again
  RESIZE: 'resize', // viewport changed mid-run (relayout can cost time)
};

/**
 * A recorder for one run. `mark()` is the only clock, so every event in a run
 * shares one time origin.
 */
export function createRecorder() {
  return {
    wallStart: null,
    monoStart: null,
    events: [],
    /** Keys physically held, so a keyup can be paired to its keydown. */
    down: new Map(),
  };
}

/** Milliseconds since the run's time origin. */
export function mark(rec) {
  return Math.round(performance.now() - rec.monoStart);
}

/** Start the clocks. Called on the first keystroke, not on page load. */
export function begin(rec) {
  if (rec.monoStart !== null) return;
  rec.wallStart = Date.now();
  rec.monoStart = performance.now();
}

/**
 * Record a keydown. `ctx` carries the typing state at press time — which word
 * and character position the user was at, and what was expected there — so a
 * keystroke can be located in the text without replaying the whole run.
 *
 * `code` is the PHYSICAL key (KeyG, Digit9), independent of layout, layers or
 * firmware remapping. `key` is the character produced. Storing both is what
 * lets analysis ask hand/finger/row questions on any keyboard, including a
 * split one, and lets a layout profile be declared or corrected later without
 * invalidating history.
 */
export function keyDown(rec, e, ctx) {
  const t = mark(rec);
  const ev = {
    k: KIND.DOWN,
    t,
    key: e.key,
    code: e.code || null,
    word: ctx.word,           // index of the active word
    pos: ctx.pos,             // caret position within that word
    expected: ctx.expected,   // character the text wanted here, or null past the end
    ok: ctx.ok,               // did it match
    rep: e.repeat === true,   // held-key auto-repeat, not a real press
    // Modifier state: distinguishes a deliberate Shift+key from a stray chord.
    mods: modBits(e),
  };
  rec.events.push(ev);
  // Remember the press so its release can be matched. Keyed by physical code
  // where available: two keys can produce the same character.
  rec.down.set(e.code || e.key, { t, i: rec.events.length - 1 });
  return ev;
}

/**
 * Record a keyup and close the loop on its keydown. Dwell (how long the key
 * was held) is a genuinely different signal from flight (gap between keys):
 * dwell tends to reflect certainty, flight reflects search. Both are stored
 * raw — `hold` here is the only convenience, and it is recomputable.
 */
export function keyUp(rec, e) {
  const t = mark(rec);
  const id = e.code || e.key;
  const opened = rec.down.get(id);
  rec.down.delete(id);
  rec.events.push({
    k: KIND.UP,
    t,
    key: e.key,
    code: e.code || null,
    hold: opened ? t - opened.t : null, // dwell time, ms
  });
}

/**
 * Record a non-keystroke event: focus loss, tab switch, resize. These are what
 * separate "paused to think" from "left the page" — the difference between a
 * hesitation finding and a distraction finding.
 */
export function note(rec, kind, extra = null) {
  if (rec.monoStart === null) return; // nothing to anchor to yet
  const ev = { k: kind, t: mark(rec) };
  if (extra) Object.assign(ev, extra);
  rec.events.push(ev);
}

/** Modifier state packed into one small int. */
function modBits(e) {
  return (e.shiftKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.altKey ? 4 : 0) | (e.metaKey ? 8 : 0);
}

/**
 * Environment captured once per run. None of this is interpreted here; it is
 * stored so that later analysis can control for it — comparing runs across
 * screen sizes, or noticing that accuracy drops on a different device.
 */
export function environment() {
  const nav = navigator;
  return {
    ua: nav.userAgent || null,
    platform: nav.platform || null,
    lang: nav.language || null,
    // Keyboard layout as the browser reports it, when permitted. Best-effort:
    // unsupported in several browsers, and resolved asynchronously elsewhere.
    tzOffset: new Date().getTimezoneOffset(),
    tz: Intl.DateTimeFormat().resolvedOptions().timeZone || null,
    screenW: window.screen?.width ?? null,
    screenH: window.screen?.height ?? null,
    viewportW: window.innerWidth,
    viewportH: window.innerHeight,
    dpr: window.devicePixelRatio ?? null,
    // Hardware hints: a slow machine inflates input latency.
    cores: nav.hardwareConcurrency ?? null,
    memory: nav.deviceMemory ?? null,
  };
}
