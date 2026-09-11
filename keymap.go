package main

// Physical key geometry. This is the layer that turns `KeyG` into "left hand,
// index finger, home row" — the mapping analysis needs and capture deliberately
// refuses to bake in.
//
// It lives in Go, not in the database, precisely so it can be corrected or
// swapped for another layout without touching a single stored row. Every
// per-finger number in the app is derived through here at query time.

// Finger identifiers, left to right as a touch typist counts them.
const (
	LPinky = "l-pinky"
	LRing  = "l-ring"
	LMid   = "l-mid"
	LIndex = "l-index"
	RIndex = "r-index"
	RMid   = "r-mid"
	RRing  = "r-ring"
	RPinky = "r-pinky"
	Thumb  = "thumb"
)

// KeyInfo is where a physical key sits and which finger owns it.
type KeyInfo struct {
	Finger string
	Hand   string // "left" | "right"
	Row    string // "number" | "top" | "home" | "bottom" | "space"
}

// qwertyKeys maps physical key codes (event.code) to geometry.
//
// Codes are layout-independent: `KeyA` is the key left of `KeyS` regardless of
// whether the firmware emits 'a', 'ä' or a macro. That is what makes this
// correct on a split keyboard running a custom layout — the finger assignment
// follows the position, which is what the hand actually does.
var qwertyKeys = map[string]KeyInfo{
	// number row
	"Backquote": {LPinky, "left", "number"}, "Digit1": {LPinky, "left", "number"},
	"Digit2": {LRing, "left", "number"}, "Digit3": {LMid, "left", "number"},
	"Digit4": {LIndex, "left", "number"}, "Digit5": {LIndex, "left", "number"},
	"Digit6": {RIndex, "right", "number"}, "Digit7": {RIndex, "right", "number"},
	"Digit8": {RMid, "right", "number"}, "Digit9": {RRing, "right", "number"},
	"Digit0": {RPinky, "right", "number"}, "Minus": {RPinky, "right", "number"},
	"Equal": {RPinky, "right", "number"},

	// top row
	"KeyQ": {LPinky, "left", "top"}, "KeyW": {LRing, "left", "top"},
	"KeyE": {LMid, "left", "top"}, "KeyR": {LIndex, "left", "top"},
	"KeyT": {LIndex, "left", "top"}, "KeyY": {RIndex, "right", "top"},
	"KeyU": {RIndex, "right", "top"}, "KeyI": {RMid, "right", "top"},
	"KeyO": {RRing, "right", "top"}, "KeyP": {RPinky, "right", "top"},
	"BracketLeft": {RPinky, "right", "top"}, "BracketRight": {RPinky, "right", "top"},

	// home row
	"KeyA": {LPinky, "left", "home"}, "KeyS": {LRing, "left", "home"},
	"KeyD": {LMid, "left", "home"}, "KeyF": {LIndex, "left", "home"},
	"KeyG": {LIndex, "left", "home"}, "KeyH": {RIndex, "right", "home"},
	"KeyJ": {RIndex, "right", "home"}, "KeyK": {RMid, "right", "home"},
	"KeyL": {RRing, "right", "home"}, "Semicolon": {RPinky, "right", "home"},
	"Quote": {RPinky, "right", "home"},

	// bottom row
	"KeyZ": {LPinky, "left", "bottom"}, "KeyX": {LRing, "left", "bottom"},
	"KeyC": {LMid, "left", "bottom"}, "KeyV": {LIndex, "left", "bottom"},
	"KeyB": {LIndex, "left", "bottom"}, "KeyN": {RIndex, "right", "bottom"},
	"KeyM": {RIndex, "right", "bottom"}, "Comma": {RMid, "right", "bottom"},
	"Period": {RRing, "right", "bottom"}, "Slash": {RPinky, "right", "bottom"},

	"Space": {Thumb, "either", "space"},
}

// keyGeometry returns the geometry for a physical code, and whether it is known.
func keyGeometry(code string) (KeyInfo, bool) {
	k, ok := qwertyKeys[code]
	return k, ok
}

// seedKeymap fills the key_geometry table, which lets SQL join to finger and
// hand without every query hard-coding a CASE expression. Rewritten on every
// start, so correcting the map above is enough to correct all analysis.
func (s *Store) seedKeymap() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM key_geometry`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT INTO key_geometry (code, finger, hand, row) VALUES (?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for code, k := range qwertyKeys {
		if _, err := stmt.Exec(code, k.Finger, k.Hand, k.Row); err != nil {
			return err
		}
	}
	return tx.Commit()
}
