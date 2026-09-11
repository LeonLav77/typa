import Alpine from './alpine.esm.js';
import { createTest, press, commit, backspace, stats, isDone, isRunning, toResult } from './engine.js';
import { createRenderer } from './render.js';

const PRINTABLE = /^[a-z]$/;

Alpine.data('typing', () => ({
  phase: 'idle', // idle | running | done
  live: { wpm: 0, accuracy: 100, seconds: 0 },
  result: null,
  test: null,
  renderer: null,
  ticker: 0,

  init() {
    this.renderer = createRenderer(this.$refs.text, this.$refs.caret);
    // The first test came down inside the HTML; adopt it instead of asking
    // the server for one we already have.
    this.adopt({
      id: this.$refs.text.dataset.testId || null,
      words: this.renderer.readServerWords(),
    });
    window.addEventListener('keydown', (e) => this.onKey(e));
    window.addEventListener('resize', () => this.renderer.relayout());
  },

  /** Common reset for a test that is already in the DOM. */
  adopt({ id, words }) {
    this.clear();
    this.test = createTest(words, id);
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
      const res = await fetch('/api/test', { headers: { accept: 'application/json' } });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const { id, words } = await res.json();
      this.clear();
      this.test = createTest(words, id);
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
      changed = commit(this.test);
    } else if (e.key === 'Backspace') {
      e.preventDefault();
      changed = backspace(this.test, e.shiftKey);
    } else if (e.key.length === 1 && PRINTABLE.test(e.key)) {
      changed = press(this.test, e.key);
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
    // Everything a server would need is already here; wiring is a later step.
    this.lastRun = toResult(this.test);
  },

  get progress() {
    return `${Math.min(this.test.index + (this.phase === 'done' ? 1 : 0), this.test.words.length)}/${this.test.words.length}`;
  },
  get elapsed() {
  return this.live.seconds.toFixed(1);
  },
}));

window.Alpine = Alpine;
Alpine.start();
