package main

// Word sources: the pool a test draws its tokens from.
//
// A source is exclusive — one pool per test — while `Numbers`, `Caps` and
// `Punctuation` stay composable modifiers layered on whatever it produces.
// That split is what lets "left hand only" and "laravel" be the same kind of
// thing as "words" instead of two special cases: a new language is a new entry
// in `sourceList` and nothing else changes.
//
// The hand pools live in handwords.go and are checked against the keymap by
// test, so a layout change fails the build rather than silently dealing words
// the named hand cannot reach.

// SourceID names a pool. It is stored in the run's mode tag, so these strings
// are data: renaming one silently splits a history in two.
type SourceID string

const (
	SrcWords     SourceID = "words"
	SrcLeftHand  SourceID = "left"
	SrcRightHand SourceID = "right"
	SrcLaravel   SourceID = "laravel"
	SrcGo        SourceID = "go"
	SrcPython    SourceID = "python"
)

// Source is one selectable pool.
type Source struct {
	ID    SourceID
	Label string
	Blurb string
	// Alias are the extra words the /text picker matches on.
	Alias []string
	// Code marks a programming vocabulary, which is what makes the symbols
	// modifier meaningful: there is nothing to decorate on plain English.
	Code bool
	// pool returns the bare tokens, and symbols the flavour with sigils and
	// operators. A source with no symbol flavour returns the same list.
	pool    func() []string
	symbols func() []string
}

// sourceList is the menu order on /text.
var sourceList = []Source{
	{
		ID: SrcWords, Label: "words", Blurb: "common English words",
		Alias: []string{"english", "normal"},
		pool:  func() []string { return words },
	},
	{
		ID: SrcLeftHand, Label: "left hand", Blurb: "words the left hand types alone",
		Alias: []string{"lefthand"},
		pool:  func() []string { return leftHandWords },
	},
	{
		ID: SrcRightHand, Label: "right hand", Blurb: "words the right hand types alone",
		Alias: []string{"righthand"},
		pool:  func() []string { return rightHandWords },
	},
	{
		ID: SrcLaravel, Label: "laravel", Blurb: "php and laravel vocabulary",
		Alias: []string{"php"}, Code: true,
		pool:    func() []string { return laravelWords },
		symbols: func() []string { return laravelSymbols },
	},
	{
		ID: SrcGo, Label: "go", Blurb: "go keywords and stdlib idioms",
		Alias: []string{"golang"}, Code: true,
		pool:    func() []string { return goWords },
		symbols: func() []string { return goSymbols },
	},
	{
		ID: SrcPython, Label: "python", Blurb: "python keywords and builtins",
		Alias: []string{"py"}, Code: true,
		pool:    func() []string { return pythonWords },
		symbols: func() []string { return pythonSymbols },
	},
}

// sourceByID indexes sourceList for lookup.
var sourceByID = func() map[SourceID]Source {
	m := make(map[SourceID]Source, len(sourceList))
	for _, s := range sourceList {
		m[s.ID] = s
	}
	return m
}()

// lookupSource returns the named source, falling back to plain words. An
// unknown id is a stale bookmark or a hand-edited URL, not an error worth a
// status code: dealing an ordinary test keeps the page usable.
func lookupSource(id SourceID) Source {
	if s, ok := sourceByID[id]; ok {
		return s
	}
	return sourceByID[SrcWords]
}

// tokens returns the pool for a source in the requested flavour. Symbols fall
// back to the bare pool where a source has no symbol list, so the modifier is
// always safe to leave on.
func (s Source) tokens(withSymbols bool) []string {
	if withSymbols && s.symbols != nil {
		return s.symbols()
	}
	if s.pool == nil {
		return words
	}
	return s.pool()
}
