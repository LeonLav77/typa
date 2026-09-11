package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestPickWordsCount(t *testing.T) {
	got := pickWords(WordCount)
	if len(got) != WordCount {
		t.Fatalf("got %d words, want %d", len(got), WordCount)
	}
	for _, w := range got {
		if w == "" {
			t.Fatal("empty word dealt")
		}
	}
}

func TestWordListIsCleanAndUnique(t *testing.T) {
	ok := regexp.MustCompile(`^[a-z]+$`)
	seen := make(map[string]bool, len(words))
	for _, w := range words {
		if !ok.MatchString(w) {
			t.Errorf("word %q is not lowercase letters only", w)
		}
		if seen[w] {
			t.Errorf("duplicate word %q", w)
		}
		seen[w] = true
	}
}

func TestChars(t *testing.T) {
	tt := Test{Words: []string{"go", "at"}}
	got := tt.Chars()
	want := [][]string{{"g", "o"}, {"a", "t"}}
	if len(got) != len(want) {
		t.Fatalf("got %d words, want %d", len(got), len(want))
	}
	for i := range want {
		if strings.Join(got[i], "") != strings.Join(want[i], "") {
			t.Errorf("word %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNewTestIDsAreDistinct(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		id := newTest().ID
		if seen[id] {
			t.Fatalf("duplicate test id %q", id)
		}
		seen[id] = true
	}
}

func TestIndexRendersEveryWord(t *testing.T) {
	rec := httptest.NewRecorder()
	handleIndex(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if n := strings.Count(body, `class="word"`); n != WordCount {
		t.Errorf("rendered %d words, want %d", n, WordCount)
	}
	if !strings.Contains(body, "data-test-id=") {
		t.Error("no test id in markup")
	}
	if strings.Contains(body, "<no value>") {
		t.Error("template left an unresolved value")
	}
}

func TestAPITestReturnsFullTest(t *testing.T) {
	rec := httptest.NewRecorder()
	handleNewTest(rec, httptest.NewRequest(http.MethodGet, "/api/test", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got Test
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Words) != WordCount {
		t.Errorf("got %d words, want %d", len(got.Words), WordCount)
	}
	if got.ID == "" {
		t.Error("empty test id")
	}
}
