/**
 * Keyboard menu. Hold alt and the access keys appear; press one to go there.
 *
 * Shared by the typing screen and the analytics pages, so navigation behaves
 * identically everywhere. The typing screen imports the same module rather
 * than reimplementing it, which is why this owns no page-specific state.
 */

const HOLD_MS = 120; // brief delay so a quick alt-tab never flashes the menu

export function install({ onOpen, onClose } = {}) {
  const menu = document.querySelector('[data-menu]');
  const hint = document.querySelector('[data-menu-hint]');
  if (!menu) return;

  // key -> href, read from the markup so Go stays the single source of truth
  // for what the menu contains.
  const targets = new Map(
    [...menu.querySelectorAll('a[data-key]')].map((a) => [
      a.dataset.key.toLowerCase(),
      a.getAttribute('href'),
    ]),
  );

  let open = false;
  let timer = 0;

  function show() {
    if (open) return;
    open = true;
    menu.hidden = false;
    // Next frame, so the transition has a start state to animate from.
    requestAnimationFrame(() => menu.classList.add('on'));
    hint?.classList.add('faded');
    onOpen?.();
  }

  function hide() {
    clearTimeout(timer);
    if (!open) return;
    open = false;
    menu.classList.remove('on');
    hint?.classList.remove('faded');
    // Keep it in the layout until the fade finishes.
    setTimeout(() => { if (!open) menu.hidden = true; }, 140);
    onClose?.();
  }

  window.addEventListener('keydown', (e) => {
    if (e.key === 'Alt' && !e.repeat) {
      clearTimeout(timer);
      timer = setTimeout(show, HOLD_MS);
      return;
    }

    // With alt held, a mapped letter navigates. Checking altKey rather than
    // our own `open` flag means the shortcut works even during the hold delay.
    if (e.altKey) {
      const href = targets.get(e.key.toLowerCase());
      if (href) {
        e.preventDefault();
        hide();
        window.location.href = href;
      }
      return;
    }

    if (e.key === 'Escape' && open) hide();
  });

  window.addEventListener('keyup', (e) => {
    if (e.key === 'Alt') hide();
  });

  // Alt+tab and similar take focus away while alt is still down; without this
  // the menu would still be open on return.
  window.addEventListener('blur', hide);

  return { show, hide, isOpen: () => open };
}

// Analytics pages have no other keyboard behaviour, so they self-install.
// The typing screen calls install() itself, to coordinate with its key handler.
if (document.body?.classList.contains('page')) install();
