/**
 * Where you are and what you are typing on.
 *
 * Two tags travel with every run, and both exist for the same reason: not
 * every run belongs in "your" numbers. A friend trying the app, or your own
 * hands on a laptop keyboard, are different populations from you on the
 * Moonlander, and mixing them quietly corrupts every per-finger and per-key
 * aggregate downstream.
 *
 *   location  work | home | other      — where the run happened
 *   device    moonlander | normal | guest — what produced the keystrokes
 *
 * Every run is always tagged. `home` and `moonlander` are the standing
 * defaults; a real measurement replaces a default, and a manual choice
 * replaces both and sticks until changed. The cases automation cannot see
 * ("other", "someone else") are exactly the ones where being asked every time
 * would be most annoying, so the manual choice is remembered rather than
 * requested per run.
 *
 * Nothing here interprets typing data — it only labels it. The labels are
 * stored raw alongside the run, so a later change of mind about what counts
 * as "work" is a query, not a lost dataset.
 */

const LS = {
  location: 'typing:location',       // manual choice, or '' to let detection decide
  device: 'typing:device',           // manual choice, or '' to let detection decide
  geo: 'typing:geo-cache',           // last known fix, so a run never waits on GPS
  mode: 'typing:mode',               // what the text is made of
};

/* ------------------------------------------------------------------ geo */

/**
 * Work is Labin. Home is anywhere else you type from, which is why only one
 * place needs coordinates: everything outside the fence is "home" by default
 * and can be corrected by hand when it is neither.
 *
 * The radius is generous on purpose. A town-level fix from WiFi/IP lookup is
 * accurate to a few hundred metres at best, so a tight fence would flap
 * between work and home on the same desk.
 */
const WORK = { lat: 45.0856, lon: 14.1161, radiusKm: 6, name: 'Labin' };

/** Great-circle distance in km. Haversine — exact enough far below its error. */
function distanceKm(a, b) {
  const R = 6371;
  const dLat = ((b.lat - a.lat) * Math.PI) / 180;
  const dLon = ((b.lon - a.lon) * Math.PI) / 180;
  const la = (a.lat * Math.PI) / 180;
  const lb = (b.lat * Math.PI) / 180;
  const h =
    Math.sin(dLat / 2) ** 2 + Math.sin(dLon / 2) ** 2 * Math.cos(la) * Math.cos(lb);
  return 2 * R * Math.asin(Math.sqrt(h));
}

/** Which tag a fix implies. */
function classify(fix) {
  return distanceKm(fix, WORK) <= WORK.radiusKm ? 'work' : 'home';
}

// A fix is reused for a day. Position is asked for once and then cached, so
// finishing a test never blocks on the geolocation stack — the tag is applied
// from the last known fix and refreshed in the background for next time.
const GEO_TTL_MS = 24 * 60 * 60 * 1000;

function cachedFix() {
  try {
    const raw = JSON.parse(localStorage.getItem(LS.geo) || 'null');
    if (raw && Date.now() - raw.at < GEO_TTL_MS) return raw;
  } catch {
    // Unreadable cache is the same as no cache.
  }
  return null;
}

function cacheFix(fix) {
  try {
    localStorage.setItem(LS.geo, JSON.stringify({ ...fix, at: Date.now() }));
  } catch {
    // Storage blocked: the fix is simply not reused next session.
  }
}

/**
 * Ask the browser where we are. Resolves to null rather than rejecting on
 * denial or timeout: not knowing the location is an ordinary outcome, and it
 * must leave the run untagged instead of failing the submission.
 *
 * Permission, once granted, persists for the origin — so this prompts at most
 * once and is silent on every later visit.
 */
export function locate({ timeout = 8000 } = {}) {
  return new Promise((resolve) => {
    if (!navigator.geolocation) return resolve(null);
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        const fix = { lat: pos.coords.latitude, lon: pos.coords.longitude };
        cacheFix(fix);
        resolve(fix);
      },
      () => resolve(null),
      // A coarse fix is enough to tell one town from another, and asking for
      // high accuracy on a desktop only costs time for no extra certainty.
      { enableHighAccuracy: false, timeout, maximumAge: GEO_TTL_MS },
    );
  });
}

/* --------------------------------------------------------------- device */

// ZSA's USB vendor id; the Moonlander's product id under it. Other ZSA boards
// share the vendor, so vendor alone is the useful test if the Voyager or a
// future board ever joins.
const ZSA_VENDOR = 0x3297;
const MOONLANDER_PRODUCT = 0x1969;

export const hasHID = () => 'hid' in navigator;

/**
 * Is a ZSA board currently attached?
 *
 * `getDevices()` returns only devices this origin was already granted, and it
 * needs no prompt and no user gesture — so after one grant this is a silent
 * check on every load. Before that grant it returns nothing, which reads as
 * "no Moonlander" and is why the pairing step below exists.
 *
 * Presence is not proof of use: with both keyboards plugged in, this says the
 * Moonlander is there, not that it typed. That is the honest limit of what a
 * web page can see, and the reason a manual override always wins.
 */
export async function moonlanderAttached() {
  if (!hasHID()) return false;
  try {
    const devices = await navigator.hid.getDevices();
    return devices.some((d) => d.vendorId === ZSA_VENDOR);
  } catch {
    return false;
  }
}

/**
 * One-time pairing. Must be called from a real click: `requestDevice` needs
 * transient user activation and shows a chooser. Afterwards the grant is
 * remembered for this origin and `moonlanderAttached()` works silently.
 *
 * Two filters: the QMK raw-HID collection when the firmware exposes it, and
 * the bare vendor id otherwise. The keyboard's own typing collection is
 * blocklisted by the browser and deliberately not requested — this only ever
 * asks whether the board is present, never what it is sending.
 */
export async function pairMoonlander() {
  if (!hasHID()) throw new Error('WebHID unavailable in this browser');
  const devices = await navigator.hid.requestDevice({
    filters: [
      { vendorId: ZSA_VENDOR, usagePage: 0xff60, usage: 0x61 },
      { vendorId: ZSA_VENDOR },
    ],
  });
  return devices.length > 0;
}

/* -------------------------------------------------------------- resolve */

const read = (k) => {
  try {
    return localStorage.getItem(k) || '';
  } catch {
    return '';
  }
};

const write = (k, v) => {
  try {
    if (v) localStorage.setItem(k, v);
    else localStorage.removeItem(k);
  } catch {
    // Private mode: the override lasts for this page only.
  }
};

export const overrides = () => ({ location: read(LS.location), device: read(LS.device) });

/** Set or clear a manual choice. Empty string hands the tag back to detection. */
export function setOverride(kind, value) {
  if (kind === 'location') write(LS.location, value);
  if (kind === 'device') write(LS.device, value);
}

/**
 * The standing defaults. Every run is tagged with something, because an
 * untagged run is a run that has to be explained later — and the overwhelmingly
 * common case is the one worth assuming.
 */
const DEFAULTS = { location: 'home', device: 'moonlander' };

/**
 * The tags to attach to a run, with how each was decided.
 *
 * Three tiers, most specific first:
 *
 *   manual   you said so on /setup. Final, and sticky until changed.
 *   geo/hid  something was actually measured, which replaces the default.
 *   default  nothing was measured, so the standing assumption stands.
 *
 * `src` records which tier won, so "was this measured or assumed?" stays
 * answerable afterwards rather than being lost in the tag itself. That
 * distinction is the whole reason a default is safe: a `default`-sourced tag
 * can be discounted later, a silently-wrong `hid` one could not.
 *
 * This never awaits a fresh fix. It uses the cached one and kicks off a
 * refresh for next time, because a finished run must be submitted immediately;
 * waiting on geolocation to label it would stall the results screen.
 */
export function resolve() {
  const o = overrides();
  const out = {};

  if (o.location) {
    out.location = o.location;
    out.locationSrc = 'manual';
  } else {
    const fix = cachedFix();
    if (fix) {
      out.location = classify(fix);
      out.locationSrc = 'geo';
    } else {
      out.location = DEFAULTS.location;
      out.locationSrc = 'default';
    }
  }

  if (o.device) {
    out.device = o.device;
    out.deviceSrc = 'manual';
  } else if (detected.device) {
    out.device = detected.device;
    out.deviceSrc = 'hid';
  } else {
    out.device = DEFAULTS.device;
    out.deviceSrc = 'default';
  }
  return out;
}

// Last known detection, refreshed in the background. Kept as module state so
// `resolve()` stays synchronous and a run is never delayed by a device probe.
const detected = { device: '' };

/**
 * Start background detection. Called once on load: it refreshes the geo fix
 * for next time, probes for the keyboard now, and keeps the device tag current
 * as boards are plugged and unplugged mid-session.
 *
 * `onChange` fires whenever the resolved tags might have changed, so the UI
 * can redraw its indicator without polling.
 */
export function watch(onChange = () => {}) {
  const refresh = async () => {
    detected.device = (await moonlanderAttached()) ? 'moonlander' : 'normal';
    onChange(resolve());
  };

  // Chrome's HID service can return an empty list very early in startup while
  // it is still reading report descriptors, so this is retried once shortly
  // after load rather than trusted at t=0.
  refresh();
  setTimeout(refresh, 1500);

  if (hasHID()) {
    navigator.hid.addEventListener('connect', refresh);
    navigator.hid.addEventListener('disconnect', refresh);
  }

  // Refresh the cached fix for the next run. The current run is tagged from
  // whatever is already cached, so this never blocks anything.
  if (!cachedFix()) locate().then(() => onChange(resolve()));

  return refresh;
}

/** Human-readable summary for the indicator line. */
export function describe(tags = resolve()) {
  const dev = DEVICE_LABELS[tags.device] || tags.device;
  return `${tags.location} · ${dev}`;
}

const DEVICE_LABELS = {
  moonlander: 'moonlander',
  normal: 'normal keyboard',
  guest: 'someone else',
};

/**
 * Whether either tag was actually chosen, as opposed to measured or assumed.
 * The indicator uses this: a stale override you forgot to change back is the
 * one failure mode that silently mislabels a whole session.
 */
export function isManual(tags = resolve()) {
  return tags.locationSrc === 'manual' || tags.deviceSrc === 'manual';
}

export const WORK_PLACE = WORK.name;

/* ----------------------------------------------------------------- mode */

/**
 * What the text is made of.
 *
 * `words` and `numbers` are sources and compose; `caps` and `punctuation`
 * decorate whatever those produce. Turning words off and numbers on therefore
 * gives a digits-only test without needing a mode named "numbers only" — the
 * combination falls out rather than being enumerated.
 *
 * The server owns generation, so this only records the choice and puts it on
 * the query string. Keeping it here rather than in the URL alone means the
 * choice survives a fresh visit.
 */
const MODE_DEFAULT = {
  words: true,
  numbers: false,
  caps: false,
  punctuation: false,
  // How much of each, 1..5. Only meaningful while the matching flag is on,
  // and kept when it is off so toggling does not lose the setting.
  numberLevel: 3,
  capLevel: 3,
  punctLevel: 3,
};

// Which level belongs to which flag. Also the set of sliders the UI draws.
export const LEVEL_OF = {
  numbers: 'numberLevel',
  caps: 'capLevel',
  punctuation: 'punctLevel',
};

export const LEVELS = 5;

export function mode() {
  try {
    const raw = JSON.parse(localStorage.getItem(LS.mode) || 'null');
    if (raw && typeof raw === 'object') return { ...MODE_DEFAULT, ...raw };
  } catch {
    // Unreadable setting is the same as none.
  }
  return { ...MODE_DEFAULT };
}

/**
 * Flip one flag. A mode with no source left on cannot produce text, so turning
 * off the last one turns the other source on instead of dealing a blank test.
 */
export function toggleMode(key) {
  const m = mode();
  m[key] = !m[key];
  if (!m.words && !m.numbers) {
    m[key === 'words' ? 'numbers' : 'words'] = true;
  }
  try {
    localStorage.setItem(LS.mode, JSON.stringify(m));
  } catch {
    // Private mode: the choice lasts for this page only.
  }
  return m;
}

/**
 * Set one slider, 1..LEVELS. Out-of-range values are clamped rather than
 * rejected, since the only caller is a control that cannot exceed its own
 * bounds and a stored value from an older build might.
 */
export function setLevel(key, n) {
  const m = mode();
  m[key] = Math.min(LEVELS, Math.max(1, Math.round(n) || 1));
  try {
    localStorage.setItem(LS.mode, JSON.stringify(m));
  } catch {
    // Private mode: the choice lasts for this page only.
  }
  return m;
}

/** The mode as query parameters, for GET / and GET /api/test. */
export function modeQuery(m = mode()) {
  return new URLSearchParams({
    words: String(m.words),
    numbers: String(m.numbers),
    caps: String(m.caps),
    punctuation: String(m.punctuation),
    numberLevel: String(m.numberLevel),
    capLevel: String(m.capLevel),
    punctLevel: String(m.punctLevel),
  }).toString();
}

// The order the server writes its mode tag in. Kept in step with Mode.String()
// in mode.go, since the two are compared to decide whether the first
// server-rendered test matches the saved setting.
const MODE_ORDER = ['words', 'numbers', 'caps', 'punctuation'];

/**
 * The server's tag for a mode, e.g. "words-numbers5".
 *
 * Must match Mode.String() in mode.go exactly: the two are compared to decide
 * whether the server-rendered first test already matches the saved setting,
 * and a mismatch silently costs an extra fetch on every load.
 */
export function modeTag(m = mode()) {
  const on = MODE_ORDER.filter((k) => m[k]).map((k) => {
    const lv = LEVEL_OF[k] ? m[LEVEL_OF[k]] : 0;
    return lv && lv !== 3 ? `${k}${lv}` : k;
  });
  return on.length ? on.join('-') : 'words';
}

/** Human-readable, for the indicator. */
export function describeMode(m = mode()) {
  const on = MODE_ORDER.filter((k) => m[k]);
  return on.length ? on.join(' + ') : 'words';
}
