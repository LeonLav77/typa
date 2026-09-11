package main

import "math/rand"

// WordCount is how many words a single test deals.
const WordCount = 50

// Test is one dealt typing test. The server owns the words; the browser owns
// what gets typed into them.
type Test struct {
	ID    string   `json:"id"`
	Words []string `json:"words"`
}

// Chars splits each word into its characters, because html/template cannot
// range over a string. Used only by the page template, never by the JSON API.
func (t Test) Chars() [][]string {
	out := make([][]string, len(t.Words))
	for i, w := range t.Words {
		chars := make([]string, 0, len(w))
		for _, c := range w {
			chars = append(chars, string(c))
		}
		out[i] = chars
	}
	return out
}

// pickWords picks n words at random, with replacement, matching the original
// client behaviour.
func pickWords(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = words[rand.Intn(len(words))]
	}
	return out
}

// newTest deals a fresh test.
func newTest() Test {
	return Test{ID: randomID(), Words: pickWords(WordCount)}
}

const idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// randomID is a short opaque handle for one dealt test, so a later results
// endpoint can tie a submission back to the words that were served.
func randomID() string {
	b := make([]byte, 12)
	for i := range b {
		b[i] = idAlphabet[rand.Intn(len(idAlphabet))]
	}
	return string(b)
}
