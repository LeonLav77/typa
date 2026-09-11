import Alpine from './alpine.esm.js';
import { createTest, press, commit, backspace, stats, isDone, isRunning, toResult } from './engine.js';
import { note, keyUp, KIND } from './capture.js';
import { createRenderer } from './render.js';
import { install as installMenu } from './menu.js';
import { submit, flush } from './report.js';
import { load as loadInsights, runNotes } from './insights.js';
import {
  resolve as resolveContext,
  watch as watchContext,
  describe,
  isManual,
  modeQuery,
  modeTag,
  describeMode,
} from './context.js';

// Any single printable character. This was `[a-z]` while every test was
// lowercase words; with numbers, capitals and punctuation available it must
// accept whatever the text can ask for, or those characters are untypeable.
// Space is excluded because it commits a word rather than being typed into one.
const PRINTABLE = /^[^\s]$/;

Alpine.data('typing', () => ({
  phase: 'idle', // idle | running | done
  setup: '',        // "work · moonlander" — what this run will be tagged with
  setupHeld: false, // true when that came from a manual choice, not detection
  live: { wpm: 0, accuracy: 100, seconds: 0 },
  result: null,
  insights: null,   // raw analytics payload
  notes: [],        // observations about the run just finished
  test: null,
  renderer: null,
  ticker: 0,

  init() {
    // Stable per-browser id so runs form a history. Not an account: it exists
    // purely to let longitudinal analysis ("you improved on 'th' this month")
    // group runs together.
    this.typist = typistID();
    // Declared keyboard profile, stored with every run. Physical key codes are
    // captured regardless; this only tells analysis how to map them to hands
    // and fingers, and can be corrected later without invalidating history.
    this.layout = localStorage.getItem('typing:layout') || 'split-qwerty';
    this.renderer = createRenderer(this.$refs.text, this.$refs.caret);
    // The first test came down inside the HTML; adopt it instead of asking
    // the server for one we already have.
    this.adopt({
      id: this.$refs.text.dataset.testId || null,
      mode: this.$refs.text.dataset.mode || '',
      words: this.renderer.readServerWords(),
    });
    // The server rendered the first test before it could know the saved mode,
    // which lives in localStorage. If they disagree, quietly deal a matching
    // one rather than making the first test of every visit the wrong kind.
    if ((this.$refs.text.dataset.mode || 'words') !== modeTag()) {
      this.reset();
    }
    // The menu owns alt; onKey already ignores alt-modified keys, so the two
    // cannot both act on one keystroke.
    installMenu();
    // Keep the setup line current as keyboards are plugged in or unplugged,
    // and as the first location fix arrives.
    const showSetup = (tags) => {
      // The text kind leads: it is the thing that changes what you are about
      // to type, where the other two only label it.
      this.setup = `${describeMode()} · ${describe(tags)}`;
      // A manual choice is the one that can go stale without any outward
      // sign, so the indicator marks it rather than reading identically to a
      // detected tag.
      this.setupHeld = isManual(tags);
    };
    showSetup();
    watchContext(showSetup);
    window.addEventListener('keydown', (e) => this.onKey(e));
    // Keyup gives dwell time, and focus/visibility events are what separate
    // "paused to think" from "left the page". Recorded always; interpreted later.
    window.addEventListener('keyup', (e) => {
      if (this.test) keyUp(this.test.rec, e);
    });
    window.addEventListener('blur', () => this.test && note(this.test.rec, KIND.BLUR));
    window.addEventListener('focus', () => this.test && note(this.test.rec, KIND.FOCUS));
    document.addEventListener('visibilitychange', () => {
      if (this.test) note(this.test.rec, document.hidden ? KIND.HIDE : KIND.SHOW);
    });
    window.addEventListener('resize', () => {
      this.renderer.relayout();
      if (this.test) {
        note(this.test.rec, KIND.RESIZE, { w: window.innerWidth, h: window.innerHeight });
      }
    });
  },

  /** Common reset for a test that is already in the DOM. */
  adopt({ id, words, mode }) {
    this.clear();
    this.test = createTest(words, id, mode);
    this.renderer.adopt(this.test);
  },

  clear() {
    clearInterval(this.ticker);
    this.phase = 'idle';
    this.result = null;
    this.live = { wpm: 0, accuracy: 100, seconds: 0 };
  },

  /** Deal a new test from the server. Falls back to reshuffling nothing —
   *  on failure the current text simply stays put. */
  async reset() {
    try {
      // The mode goes with the request: the server owns generation, so asking
      // for a different kind of text is a different query, not a client-side
      // transformation of words it already has.
      const res = await fetch(`/api/test?${modeQuery()}`, {
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const { id, words, mode } = await res.json();
      this.clear();
      this.test = createTest(words, id, mode);
      this.renderer.mount(this.test);
    } catch (err) {
      console.error('could not deal a new test:', err);
    }
  },

  onKey(e) {
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    if (e.key === 'Escape') {
      e.preventDefault();
      this.reset();
      return;
    }
    if (this.phase === 'done') {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        this.reset();
      }
      return;
    }

    const from = this.test.index;
    let changed = false;

    if (e.key === ' ') {
      e.preventDefault();
      changed = commit(this.test, e);
    } else if (e.key === 'Backspace') {
      e.preventDefault();
      changed = backspace(this.test, e, e.shiftKey);
    } else if (e.key.length === 1 && PRINTABLE.test(e.key)) {
      changed = press(this.test, e);
    }
    if (!changed) return;

    if (this.phase === 'idle' && isRunning(this.test)) this.begin();
    this.renderer.update(from);
    if (isDone(this.test)) this.end();
  },

  begin() {
    this.phase = 'running';
    this.ticker = setInterval(() => {
      const s = stats(this.test);
      this.live = { wpm: s.wpm, accuracy: s.accuracy, seconds: s.seconds };
    }, 250);
  },

  end() {
    clearInterval(this.ticker);
    this.phase = 'done';
    this.result = stats(this.test);
    this.lastRun = toResult(this.test, this.layout, resolveContext());
    this.lastRun.typist = this.typist;
    // Submit, then read back with the id the server assigned, so the results
    // screen can show this run's own numbers alongside the lifetime picture.
    submit(this.lastRun).then((runID) => this.refreshInsights(runID));
  },

  /** Fetch analytics and rebuild the results panel. Never blocks the UI. */
  async refreshInsights(runID) {
    try {
      const d = await loadInsights(this.typist, runID);
      this.insights = d;
      this.notes = runNotes(d);
    } catch (err) {
      console.error('could not load insights:', err);
    }
  },

  get progress() {
    return `${Math.min(this.test.index + (this.phase === 'done' ? 1 : 0), this.test.words.length)}/${this.test.words.length}`;
  },
  get elapsed() {
  return this.live.seconds.toFixed(1);
  },
}));

/** Stable, anonymous, per-browser identifier. */
function typistID() {
  const KEY = 'typing:typist';
  try {
    let id = localStorage.getItem(KEY);
    if (!id) {
      id = (crypto.randomUUID?.() ?? String(Date.now()) + Math.random().toString(36).slice(2));
      localStorage.setItem(KEY, id);
    }
    return id;
  } catch {
    return null; // private mode: runs still record, they just don't group
  }
}

// Retry any runs a previous session failed to deliver.
flush();

window.Alpine = Alpine;
Alpine.start();
