package main

// One-handed word pools.
//
// These are their own lists rather than a filter over `words`, because
// filtering the common-word list yields 54 left-hand words and only NINE for
// the right: QWERTY puts a and e under the left hand, so ordinary English is
// almost never right-hand-only. A 50-word test drawn from nine words is a
// memorisation drill, not a typing test.
//
// The two are built differently, and asymmetrically on purpose:
//
//   left  — dictionary words restricted to the left-hand letters, kept only
//           where the word or its stem appears in `words`. Strict, because the
//           left hand has enough common vocabulary to afford it.
//   right — curated by hand. The same filter leaves nine words, since the
//           right hand owns no vowel but u, i, o and y. Every entry is a real
//           word; they are simply less common, which is the price of the mode
//           existing at all.
//
// TestHandWordsMatchKeymap checks every entry really is typable by the hand
// that claims it, so a keymap change fails the build rather than silently
// dealing words the hand cannot reach.

// leftHandWords use only qwert/asdfg/zxcvb.
var leftHandWords = []string{
	"acted", "add", "added", "adder", "adders", "adds", "after", "age",
	"aged", "ages", "agree", "agreed", "agrees", "area", "areas", "arts",
	"bad", "base", "based", "baser", "bases", "basest", "bed", "beds",
	"better", "bettered", "betters", "car", "card", "carded", "cards", "care",
	"cared", "career", "careered", "careers", "cares", "cars", "cased",
	"cases", "create", "created", "creates", "data", "dead", "deader",
	"deadest", "decade", "decades", "degree", "degrees", "draw", "drawer",
	"drawers", "draws", "eat", "eater", "eaters", "eats", "effect",
	"effected", "effects", "face", "faced", "faces", "fact", "facts", "few",
	"fewer", "fewest", "free", "freed", "freer", "frees", "freest", "get",
	"gets", "great", "greater", "greatest", "greats", "race", "raced",
	"racer", "racers", "races", "rate", "rated", "rates", "read", "reader",
	"readers", "reads", "reds", "rest", "rested", "rests", "save", "saved",
	"saver", "savers", "saves", "seat", "seated", "seats", "see", "seed",
	"sees", "serve", "served", "server", "servers", "serves", "sex", "sexed",
	"sexes", "staff", "staffed", "staffer", "staffers", "staffs", "stage",
	"staged", "stages", "star", "stared", "stares", "stars", "state",
	"stated", "stater", "states", "street", "streets", "tax", "taxed",
	"taxes", "test", "tested", "tester", "testers", "testes", "tests",
	"trade", "traded", "trader", "traders", "trades", "tree", "treed",
	"trees", "war", "wares", "wars", "water", "watered", "wear", "wearer",
	"wearers", "wears",
}

// rightHandWords use only yuiop/hjkl/nm.
var rightHandWords = []string{
	"hill", "him", "hip", "hippo", "holy", "honk", "hook", "hoop", "hop",
	"hulk", "hum", "hump", "hunk", "hymn", "ilk", "ill", "imply", "ink",
	"inn", "ion", "join", "jolly", "joy", "jump", "junk", "kill",
	"kiln", "kilo", "kin", "kink", "knoll", "limo", "limp", "link", "lion",
	"lip", "loin", "look", "loom", "loon", "loop", "lull", "lump", "milk",
	"mill", "million", "mini", "minimum", "mink", "mom", "monk", "mono",
	"moon", "mop", "nil", "nip", "noon", "noun", "null", "nun", "nylon",
	"oil", "oink", "onion", "only", "opinion", "pill", "pin", "pink", "pinky",
	"pinup", "pony", "pool", "pop", "pull", "pulp", "pump", "pumpkin", "pun",
	"punk", "puny", "pupil", "puppy", "pylon", "unhook", "unpin", "uphill",
	"upon", "yolk", "yum",
}
