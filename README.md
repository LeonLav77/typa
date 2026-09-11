# typing

A fast, keyboard-only typing test. 50 tokens of whatever you ask for —
words, numbers, capitals, punctuation — justified. Go server, no build step,
single binary.

## Run

```
go run .                      # http://localhost:8080
go run . -addr :3000          # somewhere else
go build -o typing .          # one self-contained binary, assets embedded
go test ./...
```

There is no node toolchain. `templates/` and `static/` are compiled into the
binary with `//go:embed`, so the built executable is the whole deployment.

## Rules

- Type. A wrong character is marked red but never blocks you.
- What the text contains is set on `/text`; the default is plain words.
- `space` commits the current word right or wrong and moves on.
- `backspace` edits the current word only; `shift+backspace` clears it.
- `esc` throws the text away and deals a new one.
- The test ends on the last character of the last word; `enter` starts a new one.
- The clock starts on your first keystroke.

## Layout

| file | role |
| --- | --- |
| `main.go` | routes, embedded assets, template rendering. |
| `engine.go` | deals a test: token selection and the id that names it. |
| `mode.go` | what a test is made of, and the generator that builds it. |
| `words.go` | ~750 common English words. |
| `templates/index.html` | the page, with the first test already rendered into it. |
| `static/engine.js` | pure client state: input, keystroke log, WPM/accuracy. No DOM. |
| `static/render.js` | imperative DOM for the 50 words; patches only the active word. |
| `static/main.js` | Alpine component: key routing, phase, live meters, results. |
| `static/capture.js` | raw capture: keydown/keyup, focus, environment. Decides nothing. |
| `static/report.js` | ships finished runs to the server; queues and retries on failure. |
| `static/insights.js` | the only place that interprets data: findings and charts. |
| `static/context.js` | text mode, where you are, and what you type on. |
| `static/picker.js` | type-to-choose, shared by both settings screens. |
| `static/text.js` | `/text`: what the test is made of. |
| `static/setup.js` | `/setup`: location and keyboard. |
| `schema.sql` | storage: one row per key event. |
| `analysis.sql` | views that turn the raw stream into answers. |
| `store.go` | writes runs verbatim, in one transaction. |
| `insights.go` | reads the analysis views for `/api/insights`. |

## Who owns what

The server owns the words. `GET /` renders a dealt test straight into the
markup, so the first paint needs no round trip and the client never ships a
word list. `esc` and `enter` fetch a replacement from `GET /api/test` as JSON
rather than reloading the page.

The browser owns what happens to those words — keystrokes, timing and stats
stay client-side, because a round trip per keypress is the one thing that
would make this slow. For the same reason Alpine deliberately does not own the
text block: re-rendering 50 words on every keypress is too much work, so
`render.js` patches the active word by hand.

On first load `render.js` adopts the markup Go already produced instead of
rebuilding it; only tests fetched later are built in JS.

`html/template` cannot range over a string, so `Test.Chars()` splits each word
into characters for the template. It is used by the page only, never the API.

## Navigation

Hold <kbd>alt</kbd> and the menu appears with an access key beside each
destination; press the letter to go there. Release alt to dismiss. The same
menu is on every page, including the test itself.

| key | page | holds |
| --- | --- | --- |
| `T` | `/` | the typing test |
| `O` | `/overview` | what the data says about you, in sentences |
| `K` | `/keys` | per key, finger, hand, row, dwell |
| `E` | `/errors` | confusion matrix, recovery, error position, hard words |
| `R` | `/rhythm` | pause bands, hand transitions, row jumps, consistency, bigrams, trigrams |
| `W` | `/words` | word length, drift by position, pace, cleanest words |
| `H` | `/history` | recent runs, by day, by hour, by weekday |
| `X` | `/text` | what the test is made of |
| `S` | `/setup` | location and keyboard |

`sections` in `pages.go` is the single source of truth: adding an entry there
adds the route to the menu on every page, and a test asserts the access keys
are unique and every page renders.

The typing screen's key handler already ignores alt-modified keys, so holding
alt can never type a character.

### What the text is made of

Four flags, composed rather than enumerated as preset modes:

| flag | kind | effect | slider |
| --- | --- | --- | --- |
| `words` | source | random common words | — |
| `numbers` | source | 1–4 digit tokens mixed in | 8% → 55% of tokens |
| `punctuation` | modifier | sentence structure, plus bracket and quote pairs | 15% → 56% of tokens marked |
| `caps` | modifier | sentence capitals, and more besides | 14% → 44% of tokens |

Each flag but `words` carries a level, 1–5, set with a slider on `/text`. Five stops
rather than a percentage: the useful range is narrow and the ends are the
interesting part, so a free number would only invite a choice between 34% and
36% that nobody can feel. A test asserts each level is measurably denser than
the one below, since a slider whose stops feel alike is worse than none.

More punctuation also means shorter sentences, so full stops arrive more often
rather than only commas piling up inside the same long clause. Capitals at
level 1 are exactly the sentence openings; above that the level adds capitals
elsewhere, which is what gives the slider something to move once the structural
ones are already there.

The level rides with its flag in the run's mode tag (`words-numbers5`), because
two differently weighted runs are different exercises and a tag that hid that
would pool them. The default level is left off, so the common tag stays
readable.

Sources compose, so `words` + `numbers` is a mix. Turning `words` off leaves
digits only — that case falls out of the composition rather than existing as a
mode of its own, which is what stops the list needing a new entry for every
combination someone wants next.

`caps` and `punctuation` decorate what the sources produced, so `caps` alone
has no sentences to open and capitalises the occasional word instead — a flag
that silently does nothing would be worse. A mode with no source at all cannot
make text and falls back to words rather than dealing a blank screen.

Punctuation is applied as sentence structure, not sprinkled: a trailing comma
on random words drills a pattern nobody types. Wrappers are applied before the
terminal mark, so a sentence ends `('word').` and never `('word.')`, and both
halves of a pair always land on the same token — a test can never end on an
unclosed bracket. Sentences never open on a number, since a digit cannot take
the capital that would otherwise be expected there.

The flags are chosen on `/text`, which shows a live sample fetched from the
server — the generator is the server's, so the only honest preview is one it
produced. The mode rides on the query string (`/api/test?numbers=true&words=false`) and
is stored with every run, so "slower with punctuation" is a question about a
tag rather than a re-parse of the text after the fact. An absent parameter
keeps the old behaviour, so a bare `/` is the plain-words test it always was.

`PRINTABLE` in `main.js` had to widen from `[a-z]` to any non-space character
when this landed; with the old filter every digit, capital and symbol was
silently untypeable.

### Context: where, and on what

Not every run belongs in your numbers. Two tags travel with each one:

| tag | values | decided by |
| --- | --- | --- |
| `location` | `work` `home` `other` | position, against a geofence around Labin |
| `device` | `moonlander` `normal` `guest` | WebHID, by USB vendor id |

Every run is always tagged. Three tiers decide with what, most specific first:

| tier | `src` | wins when |
| --- | --- | --- |
| manual | `manual` | you chose it on `/setup`; sticky until changed |
| measured | `geo` / `hid` | something was actually detected |
| default | `default` | nothing was detected: `home` and `moonlander` |

The `src` is stored beside the tag, so "was this measured or assumed?" stays
answerable afterwards. That is what makes a default safe: a `default`-sourced
tag can be discounted later, whereas a silently-wrong `hid` one could not.

**Location** is one `getCurrentPosition` call, cached for a day. Inside the
Labin fence is work, anywhere else is home; `other` is manual only, because no
signal distinguishes "a cafe" from "home". The permission is asked once and
then persists for the origin, so later runs are tagged silently. A run is never
delayed waiting for a fix: the cached one tags it and a refresh is kicked off
for next time.

**Device** is `navigator.hid.getDevices()`, filtered to ZSA's vendor id
`0x3297`. That call needs no prompt and no gesture, but returns only devices
the origin was already granted, so `/setup` has a one-time *connect* button
that calls `requestDevice`. Afterwards detection is silent forever, and
connect/disconnect events keep it current mid-session.

Three honest limits, all deliberate:

- Presence is not use. With both keyboards plugged in, HID says the Moonlander
  is *there*, not that it typed. No browser API exposes which keyboard produced
  a keystroke, so a manual override always wins.
- On Linux, Chrome reads `/dev/hidraw*` directly. Without a udev rule for
  vendor `3297` the chooser is empty even with permission granted; `/setup`
  says so rather than failing silently.
- `navigator.keyboard.getLayoutMap()` looks like the right API and is not: it
  reports the OS layout, not the physical device, and is identical for both
  keyboards.

A manual choice is sticky — it is stored in `localStorage` and applies to every
later run until changed. That is what makes handing someone the keyboard cheap:
set `someone else`, let them type, set it back.

Both settings screens are driven from the keyboard, like everything else here,
and there are two ways to reach a row because they suit different moments:

| key | does |
| --- | --- |
| <kbd>↑</kbd> <kbd>↓</kbd> | move between rows |
| <kbd>enter</kbd> / <kbd>space</kbd> | take the row you are on |
| <kbd>←</kbd> <kbd>→</kbd> | its level, on `/text` |
| any letter | jump straight to a row |

Typing is fastest when you know what you want: every option on `/text` is one
keystroke (<kbd>w</kbd> <kbd>n</kbd> <kbd>p</kbd> <kbd>c</kbd>), and every one
on `/setup` is too except `other`, which needs <kbd>o</kbd><kbd>t</kbd> to
clear moonlander's o. The cursor is for the rest of the time, and it is why the
arrows always act on something visible rather than waiting for a query to
happen to leave exactly one match — arrows that do nothing read as broken keys.

The matching and key handling live in `picker.js` and are shared, so the two
screens behave identically. Pressing enter on the choice
already in force releases it back to detection. The indicator on the test screen
marks a manual choice with a dot, since a stale override is the one failure mode
with no other outward sign.

**Guest runs are stored in full and excluded from everything.** The exclusion
is one `WHERE` in `v_own_runs`, which `v_keydowns` builds on, so all thirty-odd
views inherit it. A filter that must be remembered in thirty places is one that
will be forgotten in one, and the failure would be silent.

Both tags are stored with *how* they were decided (`geo`/`hid`/`manual`), so
"was this measured or did I say so?" stays answerable. Neither is ever inferred
at query time — retagging is a query, and past runs keep the tag they were
given.

### Physical geometry

`keymap.go` maps physical key codes to finger, hand and row. It is what turns
`KeyG` into "left index, home row", and it is the reason per-finger analysis
works on a split keyboard with a custom layout: the assignment follows the
key's *position*, not the character the firmware emits.

It is seeded into `key_geometry` on every start, so it is derived data --
correcting the map corrects all historical analysis, since nothing about
fingers was ever stored with the keystrokes.

## Endpoints

| route | returns |
| --- | --- |
| `GET /` | the page, with a test dealt into it. |
| `GET /api/test` | `{ id, words, mode }` — a fresh test, for `esc` / `enter`. Takes the mode flags as query parameters. |
| `GET /static/…` | embedded CSS and JS. |
| `GET /keys`, `/errors`, `/rhythm`, `/words`, `/history` | analytics pages. |
| `GET /text` | what the test is made of. |
| `GET /setup` | location and keyboard, detected or set by hand. |
| `POST /api/results` | stores one finished run verbatim. |
| `GET /api/insights` | the numbers behind the results screen and `/overview`. |

## Analytics

Capture records what happened; it never records what it means. There is no
"hesitation" or "confusion" column anywhere, because those are definitions and
definitions change. Every insight is a query over the raw stream, so a question
first asked in a year can still be answered from data collected today.

What is captured per keystroke: the character, the PHYSICAL key (`event.code`,
so layers and remapping cannot disguise it), what was expected, whether it hit,
modifier state, and the times of both press and release. Around that: focus and
visibility changes, viewport resizes, and the environment.

Both clocks are kept. `startedAt` is wall-clock, which locates a run in real
time for time-of-day questions; every event offset is monotonic, so a clock
change cannot corrupt an interval.

`analysis.sql` is where interpretation lives. `v_keydowns` is the base view --
character keydowns only, with true down-to-down flight time -- and everything
else builds on it:

| view | answers |
| --- | --- |
| `v_confusions` | which key you hit when you meant another |
| `v_key_skill` | per-key accuracy and speed |
| `v_bigrams` | pairs, including double letters |
| `v_focus_by_time` | where in the clock you drift |
| `v_focus_by_word` | where in the text you drift |
| `v_error_recovery` | how much you slow down after a mistake |
| `v_clean_baseline` | your undisturbed pace, for comparison |
| `v_away` | genuine distraction, as distinct from thinking |
| `v_by_hour`, `v_progress` | time of day, and improvement over days |

Two traps worth knowing, both already handled:

- `keystrokes.seq` counts every event, keyups included, so consecutive
  keystrokes are two apart in it. Anything measuring distance in keystrokes
  must use `v_keydowns.key_no`, not `seq`.
- `ok` is nullable on purpose. `0` is a mistyped key; `NULL` means correctness
  does not apply (a backspace). Collapsing the two corrupts every accuracy
  aggregate.

### This run vs over time

Two questions, deliberately kept on different screens.

- **the results screen** reports on the test just finished, and nothing else:
  the score, and a line or two about what happened in that run. Finishing a
  test should not feel like opening a dashboard, so no lifetime numbers and no
  charts appear here -- just a pointer to where they live.
- **`/overview`** carries the claims about the typist. These are withheld
  until the evidence gates in `static/insights.js` are met (5 runs, 300
  keystrokes, and per-claim minimums such as 25 attempts before calling a key
  weak); until then the page says how many more runs are needed rather than
  showing an unexplained blank.

Conflating the two produces nonsense like "word 22 is where you pause most
often" on the evidence of a single pause in a single test.

`/api/insights?run=<id>` returns both halves; `this` is populated only when a
run id is given. The per-run views (`v_run_pace`, `v_run_words`,
`v_run_errors`, `v_run_recovery`, `v_run_baseline`, `v_run_keys`) are separate
from the lifetime ones rather than parameterised, because a view cannot take an
argument and a bolted-on WHERE would silently return per-run numbers under
lifetime names.

Thresholds -- what counts as a hesitation, how much slower is worth mentioning,
how much evidence a claim needs -- live in `static/insights.js` alone. They are
judgement calls, and keeping them out of capture is what makes them safe to
change.

## How a run reaches the database

Every keystroke is recorded client-side in `test.events` as
`{ t, key, code, expected, word, pos, ok }`, where `t` is milliseconds from the
first keystroke. At the end of a test `toResult()` in `static/engine.js`
packages the run — the server's test `id`, the words, per-word input, the full
event stream, the environment, the context tags and computed stats — into one
versioned object.

`static/report.js` POSTs it to `/api/results` and, on failure, queues it in
`localStorage` and retries on next load, so a run is never lost to a dropped
request. `store.go` writes it in a single transaction: either the whole event
stream lands or none of it does, because a half-written run would corrupt every
aggregate computed over it.

The server validates only enough to keep the data honest — timestamps, a
non-empty run, and the context tags clamped to known values — and computes
nothing. Every derived number is a query over the stored stream.

Because the run carries the `id` the server dealt, a submission can be tied
back to the words it was served.
