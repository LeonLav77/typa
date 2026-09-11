/**
 * Type-to-choose, shared by every settings page.
 *
 * The interaction is the same wherever it appears: letters narrow the
 * choices, enter takes the one that remains, backspace widens, escape clears.
 * It lives here rather than on one page because a second page needing it is
 * exactly when a copy would start drifting from the original.
 *
 * A page supplies its groups and what to do with a choice; this owns the
 * query, the matching and the key handling, and nothing else.
 */

/**
 * Choices a query matches, by prefix on the label or on any alias.
 *
 * Prefix rather than substring: typing `o` should mean "other", not every
 * label containing an o. Multi-word queries match each term independently, so
 * "normal key" narrows the same way "n" does.
 *
 * Aliases exist to keep every option reachable in one or two keystrokes; a
 * collision silently demotes an option to a longer query, so the set is worth
 * checking whenever an entry is added.
 */
export function matches(items, q) {
  const terms = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!terms.length) return [];
  return items.filter((c) => {
    const words = [c.label, ...(c.alias || [])].join(' ').toLowerCase().split(/\s+/);
    return terms.every((t) => words.some((w) => w.startsWith(t)));
  });
}

/**
 * Install the key handling for one page.
 *
 * `items()` returns the current choices, `onChoose(item)` applies one, and
 * `onChange()` is called whenever the query changes so the page can redraw.
 * The query is returned through a getter rather than passed around, because
 * only the page's own render needs it.
 *
 * `onAdjust(item, delta)` is optional: supply it and left/right act on the
 * narrowed choice, which is how a slider stays reachable from the keyboard.
 */
export function install({ items, onChoose, onChange, onAdjust }) {
  let query = '';
  // Which row the arrows act on. Typing narrows to a row; up/down walks to one
  // without typing at all. Without a cursor the arrows do nothing until a
  // query happens to leave exactly one match, which reads as a broken key.
  let cursor = 0;

  const state = () => {
    const all = items();
    const hits = matches(all, query);
    // A query that has narrowed to one row takes precedence over the cursor:
    // having just typed it, that is plainly the row the user means.
    const only = hits.length === 1 ? hits[0] : null;
    // The list the cursor walks is whatever is currently visible.
    const list = hits.length ? hits : all;
    const at = list[Math.min(cursor, list.length - 1)] || null;
    return { query, hits, only, active: only || at, list };
  };

  /** Move the cursor within the visible list, stopping at the ends. */
  function move(delta) {
    const { list } = state();
    if (!list.length) return;
    cursor = Math.min(list.length - 1, Math.max(0, Math.min(cursor, list.length - 1) + delta));
  }

  function onKey(e) {
    // Alt is the menu's, and a modifier chord is never a query.
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    // A focused control handles its own keys. Without this, an arrow on a
    // focused range input would step it natively AND here, moving two stops
    // for one press.
    const el = document.activeElement;
    if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT')) {
      return;
    }

    // Enter and space both take the active row. Space because it is the
    // habitual key for a checkbox, and this page is mostly checkboxes.
    if (e.key === 'Enter' || (e.key === ' ' && !query)) {
      const { active } = state();
      if (active) {
        e.preventDefault();
        query = '';
        onChoose(active);
      }
      return;
    }

    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      move(e.key === 'ArrowDown' ? 1 : -1);
      onChange();
      return;
    }

    // Left/right adjust the active row — the one the cursor is on, or the one
    // a query has narrowed to. Always acting on something visible is what
    // makes the keys feel connected rather than dead.
    if (onAdjust && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      const { active } = state();
      if (active) {
        e.preventDefault();
        onAdjust(active, e.key === 'ArrowRight' ? 1 : -1);
      }
      return;
    }

    if (e.key === 'Escape') {
      // Clear the query first; only a second press is free to reach anything
      // else, so escape never does two things at once.
      if (query) {
        e.preventDefault();
        query = '';
        onChange();
      }
      return;
    }

    if (e.key === 'Backspace') {
      if (query) {
        e.preventDefault();
        query = query.slice(0, -1);
        onChange();
      }
      return;
    }

    // A leading space is handled above as "take the active row"; within a
    // query it separates terms.

    if (e.key.length === 1 && /[a-z ]/i.test(e.key)) {
      e.preventDefault();
      query += e.key.toLowerCase();
      onChange();
    }
  }

  window.addEventListener('keydown', onKey);
  return {
    state,
    clear: () => {
      query = '';
    },
  };
}

/**
 * The query feedback line. It states what a query will do rather than only
 * showing the letters typed: with one match, that enter takes it; with
 * several, which they are; with none, that nothing matched.
 *
 * `hint` is the idle text, and `verb` says what enter would do to a given
 * choice — which differs between a toggle and an exclusive pick.
 */
export function queryLine({ query, hits, only, active }, { hint, verb }) {
  if (!query) {
    // With no query the cursor is what enter would take, so the idle line says
    // so rather than pretending nothing is selected.
    return `<p class="setup-hint">${hint}${
      active ? ` <span class="dim">·</span> <span class="q-ready">${escape(active.label)}</span>` : ''
    }</p>`;
  }

  const state = only
    ? `<span class="q-ready">${escape(only.label)}</span>
       <span class="dim">— <kbd>enter</kbd> ${verb(only)}</span>`
    : hits.length
      ? `<span class="dim">${hits.length} matches — keep typing</span>`
      : `<span class="q-none">no match</span>`;

  return `<p class="setup-hint on">
    <span class="q-text">${escape(query)}</span>
    <span class="q-caret"></span>
    ${state}
  </p>`;
}

/** Values reaching the DOM as HTML are escaped on principle, local or not. */
export function escape(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
  ));
}
