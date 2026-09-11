# typing

A fast, keyboard-only typing test. 50 random lowercase words, justified, no
punctuation or digits. Go server, no build step, single binary.

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
- `space` commits the current word right or wrong and moves on.
- `backspace` edits the current word only; `shift+backspace` clears it.
- `esc` throws the text away and deals a new one.
- The test ends on the last character of the last word; `enter` starts a new one.
- The clock starts on your first keystroke.

## Layout

| file | role |
| --- | --- |
| `main.go` | routes, embedded assets, template rendering. |
| `engine.go` | deals a test: word selection and the id that names it. |
| `words.go` | ~750 common English words. |
| `templates/index.html` | the page, with the first test already rendered into it. |
| `static/engine.js` | pure client state: input, keystroke log, WPM/accuracy. No DOM. |
| `static/render.js` | imperative DOM for the 50 words; patches only the active word. |
| `static/main.js` | Alpine component: key routing, phase, live meters, results. |

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

## Endpoints

| route | returns |
| --- | --- |
| `GET /` | the page, with a test dealt into it. |
| `GET /api/test` | `{ id, words }` — a fresh test, for `esc` / `enter`. |
| `GET /static/…` | embedded CSS and JS. |

## Data collection (later)

Every keystroke is recorded client-side in `test.events` as
`{ t, key, expected, word, pos, ok }`, where `t` is milliseconds from the first
keystroke. `toResult(test)` in `static/engine.js` packages a finished run —
the server's test `id`, the words, per-word input, the full event stream and
computed stats — into one versioned object, stored on `this.lastRun` at the end
of a test.

Nothing is stored or sent today. To start collecting, add a `POST /api/results`
handler and beacon that object from `end()` in `static/main.js`:

```js
navigator.sendBeacon('/api/results', JSON.stringify(this.lastRun));
```

Because the run carries the `id` the server dealt, a handler can tie a
submission back to the words it was served.
