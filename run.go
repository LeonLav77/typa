package main

import "time"

// Run is one submitted test, exactly as the browser recorded it.
//
// Pointer fields are deliberate: a missing value and a zero value mean
// different things here. `ok: 0` is a mistyped key; `ok: null` is a key where
// correctness does not apply (a backspace). Collapsing the two would quietly
// corrupt accuracy aggregates.
type Run struct {
	V         int        `json:"v"`
	ID        string     `json:"id"`     // the test id the server dealt
	Typist    string     `json:"typist"` // anonymous per-browser id
	StartedAt int64      `json:"startedAt"`
	EndedAt   int64      `json:"endedAt"`
	Words     []string   `json:"words"`
	Typed     []string   `json:"typed"`
	WordTimes []WordTime `json:"wordTimes"`
	Events    []Event    `json:"events"`
	Env       Env        `json:"env"`
	Layout    string     `json:"layout"`

	// Context tags. Both decide whether a run counts as "yours": a guest on
	// your keyboard, or your own hands on a borrowed one, are different
	// populations and must be separable after the fact.
	Mode        string `json:"mode"`        // what the text was made of
	Location    string `json:"location"`    // work | home | other
	LocationSrc string `json:"locationSrc"` // geo | manual
	Device      string `json:"device"`      // moonlander | normal | guest
	DeviceSrc   string `json:"deviceSrc"`   // hid | manual | default
	Stats       Stats  `json:"stats"`
}

// localTime resolves the run's start into the typist's own wall clock, so
// time-of-day questions ("worse after midnight") are asked in their timezone
// rather than the server's.
func (r *Run) localTime() time.Time {
	t := time.UnixMilli(r.StartedAt).UTC()
	// JS getTimezoneOffset() is minutes WEST of UTC, i.e. inverted from the
	// usual sign convention.
	return t.Add(-time.Duration(r.Env.TZOffset) * time.Minute)
}

// WordTime is when a word was entered, first and last touched, and committed.
type WordTime struct {
	Entered *int `json:"entered"`
	First   *int `json:"first"`
	Last    *int `json:"last"`
	Left    *int `json:"left"`
}

// Event is a single captured event: a key press or release, or a focus change.
type Event struct {
	Kind     string  `json:"k"`
	T        int     `json:"t"`
	Key      string  `json:"key"`
	Code     string  `json:"code"`
	Expected *string `json:"expected"`
	OK       *bool   `json:"ok"`
	Word     *int    `json:"word"`
	Pos      *int    `json:"pos"`
	Mods     *int    `json:"mods"`
	Repeat   bool    `json:"rep"`
	Hold     *int    `json:"hold"`
}

// Env is the capture environment, stored so later analysis can control for it.
type Env struct {
	UA        string   `json:"ua"`
	Platform  string   `json:"platform"`
	Lang      string   `json:"lang"`
	TZ        string   `json:"tz"`
	TZOffset  int      `json:"tzOffset"`
	ScreenW   *int     `json:"screenW"`
	ScreenH   *int     `json:"screenH"`
	ViewportW *int     `json:"viewportW"`
	ViewportH *int     `json:"viewportH"`
	DPR       *float64 `json:"dpr"`
	Cores     *int     `json:"cores"`
	Memory    *float64 `json:"memory"`
}

// Stats are the client's headline numbers. Stored for cheap listing only —
// every one is recomputable from the event stream, which is the source of truth.
type Stats struct {
	Seconds    float64 `json:"seconds"`
	WPM        int     `json:"wpm"`
	Raw        int     `json:"raw"`
	Accuracy   int     `json:"accuracy"`
	Correct    int     `json:"correct"`
	TypedChars int     `json:"typedChars"`
	WordsRight int     `json:"wordsRight"`
	Words      int     `json:"words"`
	Keystrokes int     `json:"keystrokes"`
}

// Valid tag values. Anything else is discarded rather than stored, so a typo
// in the client can never create a fourth location that silently splits every
// aggregate. Unknown is represented as an empty string, which reaches SQLite
// as NULL -- "not tagged" rather than a wrong tag.
var (
	validLocations = map[string]bool{"work": true, "home": true, "other": true}
	validDevices   = map[string]bool{"moonlander": true, "normal": true, "guest": true}
	validSources   = map[string]bool{"geo": true, "hid": true, "manual": true, "default": true}
)

// normalise clamps the context tags to known values. It is deliberately
// silent: a run with an unrecognised tag is still a real run and is worth
// storing untagged, whereas rejecting it would lose the keystrokes too.
func (r *Run) normalise() {
	if !validLocations[r.Location] {
		r.Location, r.LocationSrc = "", ""
	}
	if !validDevices[r.Device] {
		r.Device, r.DeviceSrc = "", ""
	}
	if !validSources[r.LocationSrc] {
		r.LocationSrc = ""
	}
	if !validSources[r.DeviceSrc] {
		r.DeviceSrc = ""
	}
}
