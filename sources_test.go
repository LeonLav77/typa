package main

import (
	"math/rand"
	"strings"
	"testing"
)

// Every hand pool word must really be typable by the hand that claims it,
// checked against the same keymap the per-hand statistics are derived from.
// This is what keeps handwords.go honest: change the layout and this fails,
// rather than the app quietly dealing words the hand cannot reach.
func TestHandWordsMatchKeymap(t *testing.T) {
	byLetter := map[rune]string{}
	for code, info := range qwertyKeys {
		if letter, ok := strings.CutPrefix(code, "Key"); ok && len(letter) == 1 {
			byLetter[rune(letter[0]|0x20)] = info.Hand
		}
	}

	for _, tc := range []struct {
		hand string
		pool []string
	}{
		{"left", leftHandWords},
		{"right", rightHandWords},
	} {
		if len(tc.pool) == 0 {
			t.Fatalf("%s pool is empty", tc.hand)
		}
		for _, w := range tc.pool {
			for _, c := range w {
				if h, ok := byLetter[c]; !ok || h != tc.hand {
					t.Errorf("%s pool has %q, but %q is %s", tc.hand, w, c, h)
					break
				}
			}
		}
	}
}

// A pool small enough to repeat within one test is a memorisation drill, not a
// typing test — the reason the hand pools are their own list rather than a
// filter over `words`. WordCount is the floor that keeps a test non-repeating
// in principle.
func TestPoolsAreLargeEnough(t *testing.T) {
	for _, s := range sourceList {
		for _, sym := range []bool{false, true} {
			got := s.tokens(sym)
			if len(got) < WordCount {
				t.Errorf("source %s (symbols=%v) has %d tokens, want >= %d",
					s.ID, sym, len(got), WordCount)
			}
		}
	}
}

// The engine splits a test on spaces, so a token containing one would be dealt
// as two words and every word-level timing for it would be wrong.
func TestPoolTokensAreSingleTokens(t *testing.T) {
	for _, s := range sourceList {
		for _, sym := range []bool{false, true} {
			for _, tok := range s.tokens(sym) {
				if tok == "" {
					t.Errorf("source %s (symbols=%v) has an empty token", s.ID, sym)
				}
				if strings.ContainsAny(tok, " \t\n") {
					t.Errorf("source %s (symbols=%v) token %q contains whitespace",
						s.ID, sym, tok)
				}
			}
		}
	}
}

// A test must be drawn from the pool that was asked for. Without this, a
// source could silently fall back to English and the per-mode analysis would
// compare two things that were actually the same exercise.
func TestGenerateDrawsFromChosenPool(t *testing.T) {
	for _, s := range sourceList {
		in := map[string]bool{}
		for _, w := range s.tokens(false) {
			in[w] = true
		}
		m := Mode{Words: true, Source: s.ID}
		got := generate(rand.New(rand.NewSource(1)), WordCount, m)
		if len(got) != WordCount {
			t.Fatalf("source %s: got %d words", s.ID, len(got))
		}
		for _, w := range got {
			if !in[w] {
				t.Errorf("source %s dealt %q, which is not in its pool", s.ID, w)
			}
		}
	}
}

// The mode tag is what analysis groups on, so each pool and flavour must be
// separable from the others — that is the whole point of recording it.
func TestModeTagSeparatesPools(t *testing.T) {
	for _, tc := range []struct {
		mode Mode
		want string
	}{
		{Mode{Words: true, Source: SrcWords}, "words"},
		{Mode{Words: true, Source: SrcLeftHand}, "left"},
		{Mode{Words: true, Source: SrcRightHand}, "right"},
		{Mode{Words: true, Source: SrcLaravel}, "laravel"},
		{Mode{Words: true, Source: SrcLaravel, Symbols: true}, "laravel-symbols"},
		{Mode{Words: true, Source: SrcGo, Symbols: true}, "go-symbols"},
		// Symbols on a pool that has no symbol flavour must not invent a tag
		// for an exercise that did not happen.
		{Mode{Words: true, Source: SrcWords, Symbols: true}, "words"},
		{Mode{Words: true, Source: SrcLeftHand, Symbols: true}, "left"},
		// Modifiers still compose on top of the pool.
		{Mode{Words: true, Source: SrcGo, Punctuation: true}, "go-punctuation"},
		{Mode{Words: true, Source: SrcPython, Numbers: true}, "python-numbers"},
	} {
		if got := tc.mode.String(); got != tc.want {
			t.Errorf("%#v tag = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// Tags reach analysis as SQL group keys and the client as a data attribute, so
// a stray separator or space would split one mode into two in the history.
func TestModeTagsAreCleanKeys(t *testing.T) {
	for _, s := range sourceList {
		for _, sym := range []bool{false, true} {
			tag := Mode{Words: true, Source: s.ID, Symbols: sym}.String()
			if tag == "" {
				t.Errorf("source %s produced an empty tag", s.ID)
			}
			if strings.ContainsAny(tag, " +&='\"<>") {
				t.Errorf("source %s tag %q contains a character that needs escaping", s.ID, tag)
			}
		}
	}
}

// The client builds the same tag string the server does, so a run dealt from
// the page and one dealt from a URL land in the same bucket in analysis. The
// two implementations are in different languages and drift silently, so the
// client's list is parsed out of context.js and compared.
func TestSourcesMatchClient(t *testing.T) {
	src, err := staticFS.ReadFile("static/context.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)

	for _, s := range sourceList {
		// The id as the client spells it, and whether it claims a symbol
		// flavour. Both must agree or the tags diverge.
		want := `{ id: '` + string(s.ID) + `',`
		if !strings.Contains(js, want) {
			t.Errorf("context.js has no source %q", s.ID)
			continue
		}
		entry := js[strings.Index(js, want):]
		if end := strings.Index(entry, "},"); end > 0 {
			entry = entry[:end]
		}
		clientCode := strings.Contains(entry, "code: true")
		serverCode := s.symbols != nil
		if clientCode != serverCode {
			t.Errorf("source %s: client code=%v, server symbols=%v",
				s.ID, clientCode, serverCode)
		}
	}

	// And nothing extra on the client: a pool the server does not know would
	// fall back to English while the tag claimed otherwise.
	for _, id := range []string{"words", "left", "right", "laravel", "go", "python"} {
		if _, ok := sourceByID[SourceID(id)]; !ok {
			t.Errorf("client lists %q but the server has no such source", id)
		}
	}
}

// This is a keyboard-only tool, so every control on the settings pages must be
// reachable from the picker. A bare <input> with no picker entry is a control
// that cannot be operated at all without a mouse — which is how the symbols
// toggle originally shipped.
func TestSettingsControlsAreKeyboardReachable(t *testing.T) {
	for _, page := range []string{"static/text.js", "static/setup.js"} {
		src, err := staticFS.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		js := string(src)

		// Range inputs are exempt: the picker drives them through onAdjust
		// with the left/right arrows rather than by being items themselves.
		for _, bad := range []string{`type="checkbox"`, `type="radio"`} {
			if strings.Contains(js, bad) {
				t.Errorf("%s has an %s with no picker entry: not keyboard-reachable", page, bad)
			}
		}
	}
}
