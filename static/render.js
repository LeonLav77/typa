/**
 * Imperative renderer for the text block. Alpine owns the chrome; this owns
 * the 50 words, because re-rendering them on every keystroke would not be fast.
 * Only the active word is patched per keypress.
 *
 * The first test arrives already rendered by the Go template, so `adopt()`
 * reads the words back out of the DOM rather than rebuilding it. Later tests
 * come from /api/test as JSON and are built here by `mount()`.
 */
export function createRenderer(root, caret) {
  const scroller = root.parentElement; // translated to keep the active line in view
  let test = null;
  let wordEls = [];
  let lineHeight = 0;
  let frame = 0;

  /** Read the server-rendered words out of the existing markup. */
  function readServerWords() {
    return [...root.querySelectorAll('.word')].map((el) => el.textContent);
  }

  /** Bind to markup the server already rendered — no DOM is built. */
  function adopt(t) {
    test = t;
    wordEls = [...root.querySelectorAll('.word')];
    settle();
  }

  /** Build the text block for a test fetched after page load. */
  function mount(t) {
    test = t;
    const frag = document.createDocumentFragment();
    wordEls = t.words.map((word, i) => {
      const el = document.createElement('span');
      el.className = 'word';
      el.dataset.i = String(i);
      for (const ch of word) {
        const c = document.createElement('span');
        c.className = 'char';
        c.textContent = ch;
        el.appendChild(c);
      }
      frag.appendChild(el);
      if (i < t.words.length - 1) frag.appendChild(document.createTextNode(' '));
      return el;
    });
    root.replaceChildren(frag);
    settle();
  }

  /** Shared post-(a)mount work: reset scroll, measure, paint the first word. */
  function settle() {
    scroller.style.transform = 'translateY(0)';
    lineHeight = parseFloat(getComputedStyle(root).lineHeight) || 0;
    patch(0);
    schedule();
  }

  /** Rebuild one word's characters from the typed input. */
  function patch(i) {
    const el = wordEls[i];
    if (!el) return;
    const word = test.words[i];
    const got = test.typed[i];
    const kids = el.childNodes;

    const committed = i < test.index;

    for (let c = 0; c < word.length; c++) {
      let node = kids[c];
      if (!node) {
        node = document.createElement('span');
        node.textContent = word[c];
        el.appendChild(node);
      }
      // Past the input: red once the word is behind you, otherwise still untyped.
      node.className =
        c >= got.length
          ? committed ? 'char bad' : 'char'
          : got[c] === word[c] ? 'char ok' : 'char bad';
    }
    // Overtyped characters are counted, never drawn.
    while (kids.length > word.length) el.removeChild(el.lastChild);

    el.classList.toggle('active', i === test.index);
  }

  function schedule() {
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(place);
  }

  /** Park the caret on the next character and keep the active line in view. */
  function place() {
    const el = wordEls[test.index];
    if (!el) return;
    const pos = Math.min(test.typed[test.index].length, el.childNodes.length);
    const at = el.childNodes[pos];
    const ref = at ?? el.lastChild;
    if (!ref) return;
    const x = at ? ref.offsetLeft : ref.offsetLeft + ref.offsetWidth;
    const y = ref.offsetTop;
    caret.style.transform = `translate(${x}px, ${y}px)`;
    caret.style.height = `${ref.offsetHeight}px`;

    if (lineHeight) {
      // Keep the active line parked one line below the top of a taller
      // viewport, so there is always context above and several lines of
      // lookahead below. Reads --rows from CSS so the two cannot disagree.
      const rows = parseInt(
        getComputedStyle(root).getPropertyValue('--rows'), 10,
      ) || 3;
      const keepAbove = rows >= 5 ? 2 : 1;
      const line = Math.round(y / lineHeight);
      scroller.style.transform =
        `translateY(${-Math.max(0, line - keepAbove) * lineHeight}px)`;
    }
  }

  return {
    readServerWords,
    adopt,
    mount,
    /** Called after every state change; `from` is the word that may also need repainting. */
    update(from) {
      if (from !== undefined && from !== test.index) patch(from);
      patch(test.index);
      schedule();
    },
    relayout: schedule,
  };
}
