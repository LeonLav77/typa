package main

import (
	"math/rand"
	"net/url"
	"strconv"
	"strings"
)

// Mode is what a test is made of.
//
// The flags are sources and one modifier, and they compose rather than naming
// preset modes. `Words` and `Numbers` each contribute tokens; `Caps` and
// `Punctuation` decorate whatever those produced. That is what makes "only
// numbers" fall out of turning Words off rather than existing as its own mode:
// a preset list would need a new entry for every combination, and would still
// miss the one the user wanted next.
type Mode struct {
	Words       bool `json:"words"`
	Numbers     bool `json:"numbers"`
	Caps        bool `json:"caps"`
	Punctuation bool `json:"punctuation"`

	// How much of each, 1..5, where 0 means "unset, use the default". A level
	// rather than a raw percentage: the useful range is narrow and the ends
	// are the interesting part, so five stops cover it without inviting a
	// choice between 34% and 36% that nobody can feel.
	//
	// The level only applies when its flag is on, so a stored level survives
	// toggling the flag off and back on.
	NumberLevel int `json:"numberLevel"`
	CapLevel    int `json:"capLevel"`
	PunctLevel  int `json:"punctLevel"`
}

// levels is the count of stops on each slider.
const levels = 5

// defaultLevel is the middle stop, used when a level is unset.
const defaultLevel = 3

// clampLevel keeps a level in range, treating 0 (absent) as the default.
func clampLevel(n int) int {
	if n <= 0 {
		return defaultLevel
	}
	if n > levels {
		return levels
	}
	return n
}

// numberShareAt is how often a token is a number, by level. Level 5 is a
// majority but never all: with words on, a test of pure digits is what
// switching words off is for.
var numberShareAt = [levels + 1]float64{0, 0.08, 0.14, 0.22, 0.35, 0.55}

// innerMarkAt is how often a mid-sentence mark appears, by level.
var innerMarkAt = [levels + 1]float64{0, 0.05, 0.09, 0.14, 0.22, 0.32}

// wrapAt is how often a token is wrapped in a bracket or quote pair, by level.
var wrapAt = [levels + 1]float64{0, 0.02, 0.05, 0.09, 0.15, 0.24}

// sentenceLenAt bounds sentence length by punctuation level: more punctuation
// means shorter sentences, so full stops arrive more often too.
var sentenceLenAt = [levels + 1][2]int{{0, 0}, {8, 16}, {6, 13}, {4, 11}, {3, 8}, {2, 5}}

// extraCapAt is how often a word that does NOT open a sentence is capitalised,
// by level. Sentence openings are always capitalised when caps are on; this is
// what makes the slider mean something on top of that structure.
var extraCapAt = [levels + 1]float64{0, 0.0, 0.04, 0.10, 0.20, 0.35}

// loneCapAt is the chance of a capital with punctuation OFF, where there are
// no sentences to open and the level is the only thing setting the rate.
var loneCapAt = [levels + 1]float64{0, 0.05, 0.10, 0.18, 0.30, 0.45}

// defaultMode is the test as it has always been: plain lowercase words.
func defaultMode() Mode {
	return Mode{Words: true}
}

// valid reports whether the mode can produce any text at all. Caps and
// Punctuation are modifiers, so a mode with neither source on is empty no
// matter what else is set.
func (m Mode) valid() bool { return m.Words || m.Numbers }

// modeFromQuery reads a mode off the URL. Absent parameters keep the default,
// so `/api/test` with no query behaves exactly as it did before this existed.
//
// A mode with every source switched off is not an error worth a status code:
// it is a UI that got ahead of itself, and falling back to words keeps the
// screen usable instead of blank.
func modeFromQuery(q url.Values) Mode {
	m := Mode{
		Words:       boolParam(q, "words", true),
		Numbers:     boolParam(q, "numbers", false),
		Caps:        boolParam(q, "caps", false),
		Punctuation: boolParam(q, "punctuation", false),
		NumberLevel: levelParam(q, "numberLevel"),
		CapLevel:    levelParam(q, "capLevel"),
		PunctLevel:  levelParam(q, "punctLevel"),
	}
	if !m.valid() {
		return defaultMode()
	}
	return m
}

// levelParam reads a 1..5 slider position. Anything absent or unparseable
// becomes 0, which clampLevel reads as "use the default" -- so a malformed URL
// deals an ordinary test rather than an error.
func levelParam(q url.Values, key string) int {
	n, err := strconv.Atoi(q.Get(key))
	if err != nil || n < 1 || n > levels {
		return 0
	}
	return n
}

func boolParam(q url.Values, key string, def bool) bool {
	v := q.Get(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// String is the tag stored with a run, so analysis can separate "slow at
// punctuation" from "slow in general" without re-deriving it from the text.
//
// Joined with '-' rather than '+': the tag is also written into a data
// attribute, and html/template escapes '+' to "&#43;", which silently broke a
// string comparison on the client.
func (m Mode) String() string {
	parts := make([]string, 0, 4)
	if m.Words {
		parts = append(parts, "words")
	}
	// A level rides with its flag: two runs weighted differently are different
	// exercises, and a tag that hid that would pool them in analysis. The
	// suffix is omitted at the default so the common tag stays readable.
	if m.Numbers {
		parts = append(parts, withLevel("numbers", m.NumberLevel))
	}
	if m.Caps {
		parts = append(parts, withLevel("caps", m.CapLevel))
	}
	if m.Punctuation {
		parts = append(parts, withLevel("punctuation", m.PunctLevel))
	}
	if len(parts) == 0 {
		return "words"
	}
	return strings.Join(parts, "-")
}

/* --------------------------------------------------------------- tokens */

// maxDigits bounds a generated number. Long numbers are a different exercise
// (memorising a span) from typing one, and they distort per-token timing.
const maxDigits = 4

// randomNumber returns a number of 1..maxDigits digits, never zero-padded.
func randomNumber(r *rand.Rand) string {
	n := r.Intn(maxDigits) + 1
	b := make([]byte, n)
	// First digit is 1-9 so the token reads as a real number.
	b[0] = byte('1' + r.Intn(9))
	for i := 1; i < n; i++ {
		b[i] = byte('0' + r.Intn(10))
	}
	return string(b)
}

/* ------------------------------------------------------------ generation */

// minSentence is the shortest a sentence may be at any level. It is also the
// tail below which a new sentence is not opened at all, since a one-token
// sentence reads as a bug.
const minSentence = 2

// innerMarks are the punctuation that appears mid-sentence, with the weights
// roughly reflecting how often each really occurs.
var innerMarks = []string{",", ",", ",", ";", ":", "--"}

// endMarks close a sentence.
var endMarks = []string{".", ".", ".", ".", "?", "!"}

// wrappers are the paired delimiters. Both halves are applied to one token, so
// a test never ends mid-pair with an unclosed bracket the typist cannot
// resolve.
var wrappers = [][2]string{
	{`"`, `"`}, {`'`, `'`}, {"(", ")"}, {"[", "]"}, {"{", "}"},
}

// generate builds the token list for a mode.
//
// Punctuation is applied as sentence structure rather than sprinkled: a
// trailing comma on random words trains a pattern nobody types. With caps on,
// each sentence opens with a capital, which is what makes the shift key part
// of the exercise rather than a separate mode.
func generate(r *rand.Rand, n int, m Mode) []string {
	if !m.valid() {
		m = defaultMode()
	}

	// Resolve the levels once: every rate below is a table lookup, not a
	// branch, so the loop reads the same whatever the sliders say.
	numShare := numberShareAt[clampLevel(m.NumberLevel)]
	inner := innerMarkAt[clampLevel(m.PunctLevel)]
	wrap := wrapAt[clampLevel(m.PunctLevel)]
	span := sentenceLenAt[clampLevel(m.PunctLevel)]
	extraCap := extraCapAt[clampLevel(m.CapLevel)]
	loneCap := loneCapAt[clampLevel(m.CapLevel)]

	out := make([]string, 0, n)
	// Tokens still owed to the sentence being built; 0 means the next token
	// opens a new one.
	left := 0

	for len(out) < n {
		opening := false
		if m.Punctuation && left == 0 {
			left = span[0] + r.Intn(span[1]-span[0]+1)
			opening = true
			// Don't open a sentence that cannot be finished. A single token
			// left over becomes "She." -- a capital and a full stop with
			// nothing between them, which looks like a bug rather than text.
			// Folding it into the previous sentence is the lesser evil.
			if remaining := n - len(out); remaining < minSentence {
				if len(out) > 0 {
					out[len(out)-1] = strings.TrimRight(out[len(out)-1], ".?!")
				}
				opening = len(out) == 0
			}
		}

		tok := nextToken(r, m, numShare)
		// A sentence must not open on a number: a digit cannot take a capital,
		// so "608 use network." reads as a missing capital rather than a
		// deliberate one. Re-draw a word for that position when one is
		// available.
		if m.Caps && m.Punctuation && opening && m.Words && len(tok) > 0 && isDigit(tok[0]) {
			tok = words[r.Intn(len(words))]
		}

		// Capitalise the token that opens a sentence. Without punctuation
		// there are no sentences, so caps fall back to occasional words --
		// otherwise the flag would do nothing at all on a words-only test.
		if m.Caps {
			if m.Punctuation {
				// A sentence always opens with a capital; the level adds
				// capitals elsewhere, which is what gives the slider something
				// to move once the structural ones are already there.
				if opening || r.Float64() < extraCap {
					tok = capitalise(tok)
				}
			} else if r.Float64() < loneCap {
				tok = capitalise(tok)
			}
		}

		if m.Punctuation {
			left--
			closing := left == 0 || len(out) == n-1

			// Wrap BEFORE any mark is appended, so the closing bracket or
			// quote sits inside the sentence and the full stop stays outside
			// it -- ('word.') buries the sentence end, ('word'). does not.
			if r.Float64() < wrap {
				w := wrappers[r.Intn(len(wrappers))]
				tok = w[0] + tok + w[1]
			}

			// Close the sentence when its budget runs out, and also on the
			// final token so a test never trails off mid-clause.
			if closing {
				tok += pick(r, endMarks)
				left = 0
			} else if !opening && r.Float64() < inner {
				// Never on the opening token: "Word, ..." as a sentence start
				// is not a pattern worth drilling.
				tok += pick(r, innerMarks)
			}
		}

		out = append(out, tok)
	}
	return out
}

// nextToken picks one token from the enabled sources. `numShare` is how often
// a number wins when both sources are on.
func nextToken(r *rand.Rand, m Mode, numShare float64) string {
	switch {
	case m.Words && m.Numbers:
		if r.Float64() < numShare {
			return randomNumber(r)
		}
		return words[r.Intn(len(words))]
	case m.Numbers:
		return randomNumber(r)
	default:
		return words[r.Intn(len(words))]
	}
}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// capitalise uppercases the first letter. Tokens that start with a digit are
// returned unchanged, since a number has no case to give it.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	c := s[0]
	if c >= 'a' && c <= 'z' {
		return string(c-32) + s[1:]
	}
	return s
}

// isDigit reports whether b is an ASCII digit.
func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// withLevel suffixes a name with its level, unless it is the default.
func withLevel(name string, level int) string {
	if l := clampLevel(level); l != defaultLevel {
		return name + strconv.Itoa(l)
	}
	return name
}
