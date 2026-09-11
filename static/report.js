/**
 * Ships a finished run to the server. Delivery only — no interpretation, and
 * no data is dropped or summarised on the way out.
 */

const ENDPOINT = '/api/results';

/**
 * Send one run. Resolves to the server's id for it, which the results screen
 * needs in order to ask for this run's own numbers.
 *
 * A run is never lost to a failed request: on failure it is queued in
 * localStorage and retried on next load (with no id, since it was never
 * stored).
 */
export async function submit(run) {
  const body = JSON.stringify(run);
  try {
    return await send(body);
  } catch {
    queue(body);
    return null;
  }
}

async function send(body) {
  const res = await fetch(ENDPOINT, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body,
    keepalive: true, // survives navigation away
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const { run } = await res.json();
  return run ?? null;
}

const QUEUE_KEY = 'typing:pending-runs';
const QUEUE_MAX = 50; // bound the backlog; oldest is dropped first

function queue(body) {
  try {
    const pending = JSON.parse(localStorage.getItem(QUEUE_KEY) || '[]');
    pending.push(body);
    while (pending.length > QUEUE_MAX) pending.shift();
    localStorage.setItem(QUEUE_KEY, JSON.stringify(pending));
  } catch {
    // Storage full or blocked: the run is lost rather than breaking the app.
  }
}

/** Retry anything queued by an earlier failure. Called once on load. */
export async function flush() {
  let pending;
  try {
    pending = JSON.parse(localStorage.getItem(QUEUE_KEY) || '[]');
  } catch {
    return;
  }
  if (!pending.length) return;

  const failed = [];
  for (const body of pending) {
    try {
      await send(body);
    } catch {
      failed.push(body);
    }
  }
  try {
    if (failed.length) localStorage.setItem(QUEUE_KEY, JSON.stringify(failed));
    else localStorage.removeItem(QUEUE_KEY);
  } catch {
    // ignore
  }
}
