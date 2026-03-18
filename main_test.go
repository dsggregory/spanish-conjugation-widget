package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ------------------------
// Helpers: normalize
// ------------------------

func TestNormalize_RemovesAccentsAndWhitespace(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  Café  ", "cafe"},
		{"CAFE", "cafe"},
		{"ca\t fé\n", "ca fe"},
		{"  me  quedé  ", "me quede"},
	}
	for _, c := range cases {
		got := normalize(c.in)
		if got != c.want {
			t.Fatalf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ------------------------
// collectTenses
// ------------------------

func TestCollectTensesUniqueOrder(t *testing.T) {
	ss := []Sentence{
		{Blanks: []Blank{{Tense: "preterite"}, {Tense: "imperfect"}}},
		{Blanks: []Blank{{Tense: "preterite"}, {Tense: "future"}}},
		{Blanks: []Blank{{Tense: ""}}}, // empty should map to present by default
	}
	got := collectTenses(ss)
	want := []string{"preterite", "imperfect", "future", "present"}
	if len(got) != len(want) {
		t.Fatalf("collectTenses length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("collectTenses[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// ------------------------
// Tense visual helpers
// ------------------------

func TestTenseHelpersHaveFallbacks(t *testing.T) {
	// Unknown tense should not panic and should produce sensible fallback values
	unk := "some_new_tense"
	c := tenseBadgeClass(unk)
	if c == "" {
		t.Fatal("tenseBadgeClass returned empty for unknown tense")
	}
	lbl := tenseBadgeLabel(unk)
	if !strings.Contains(lbl, unk) {
		t.Fatalf("tenseBadgeLabel(%q) = %q, want it to contain the key", unk, lbl)
	}
	desc := tenseLegendDesc(unk)
	if desc == "" {
		t.Fatal("tenseLegendDesc returned empty for unknown tense")
	}
}

// ------------------------
// isOverviewEmpty
// ------------------------

func TestIsOverviewEmptyCases(t *testing.T) {
	var nilOv *Overview
	if !isOverviewEmpty(nilOv) {
		t.Fatal("nil overview should be empty")
	}
	if !isOverviewEmpty(&Overview{}) {
		t.Fatal("zero overview should be empty")
	}
	if isOverviewEmpty(&Overview{Title: "x"}) {
		t.Fatal("title should make overview non-empty")
	}
	if isOverviewEmpty(&Overview{Topic: "x"}) {
		t.Fatal("topic should make overview non-empty")
	}
	if isOverviewEmpty(&Overview{Translation: "x"}) {
		t.Fatal("translation should make overview non-empty")
	}
	if isOverviewEmpty(&Overview{Tenses: []string{"present"}}) {
		t.Fatal("tenses should make overview non-empty")
	}
	if isOverviewEmpty(&Overview{Notes: []Note{{Heading: "H"}}}) {
		t.Fatal("non-empty note heading should make overview non-empty")
	}
	if isOverviewEmpty(&Overview{Notes: []Note{{Body: "B"}}}) {
		t.Fatal("non-empty note body should make overview non-empty")
	}
	if !isOverviewEmpty(&Overview{Notes: []Note{{Heading: " ", Body: " "}}}) {
		t.Fatal("blank notes should be treated as empty")
	}
}

// ------------------------
// Story loading compatibility
// ------------------------

func TestLoadStoryFromBytes_CurrentFormat(t *testing.T) {
	t.Cleanup(func() { currentStory = Story{} })
	js := `{"overview":{"title":"t"},"sentences":[{"id":1,"translation":"x","parts":["a","b"],"blanks":[{"infinitive":"ir","answer":"voy","tense":"present","hint":""}],"rationale":[""]}]}`
	if err := loadStoryFromBytes([]byte(js)); err != nil {
		t.Fatalf("loadStoryFromBytes current format failed: %v", err)
	}
	if currentStory.Overview == nil || currentStory.Overview.Title != "t" {
		t.Fatalf("overview not loaded correctly: %+v", currentStory.Overview)
	}
	if len(currentStory.Sentences) != 1 {
		t.Fatalf("sentences not loaded: %d", len(currentStory.Sentences))
	}
}

func TestLoadStoryFromBytes_LegacyStringOverview(t *testing.T) {
	t.Cleanup(func() { currentStory = Story{} })
	js := `{"overview":"Simple text","sentences":[{"id":0,"translation":"x","parts":["a","b"],"blanks":[{"infinitive":"ir","answer":"voy","tense":"present","hint":""}],"rationale":[""]}]}`
	if err := loadStoryFromBytes([]byte(js)); err != nil {
		t.Fatalf("loadStoryFromBytes legacy v1 failed: %v", err)
	}
	if currentStory.Overview == nil || len(currentStory.Overview.Notes) != 1 {
		t.Fatalf("legacy overview not converted to note: %+v", currentStory.Overview)
	}
	if head := currentStory.Overview.Notes[0].Heading; head == "" {
		t.Fatalf("expected default note heading, got empty")
	}
}

func TestLoadStoryFromBytes_RootArray(t *testing.T) {
	t.Cleanup(func() { currentStory = Story{} })
	js := `[{"id":2,"translation":"x","parts":["a","b"],"blanks":[{"infinitive":"ser","answer":"soy","tense":"present","hint":""}],"rationale":[""]}]`
	if err := loadStoryFromBytes([]byte(js)); err != nil {
		t.Fatalf("loadStoryFromBytes root array failed: %v", err)
	}
	if currentStory.Overview != nil {
		t.Fatalf("expected nil overview for root array, got: %+v", currentStory.Overview)
	}
	if len(currentStory.Sentences) != 1 || currentStory.Sentences[0].ID != 2 {
		t.Fatalf("sentences not loaded correctly: %+v", currentStory.Sentences)
	}
}

// ------------------------
// Handlers: /api/sentences and /api/check
// ------------------------

func setupTestStory() {
	currentStory = Story{
		Overview: nil,
		Sentences: []Sentence{
			{
				ID:          42,
				Translation: "x",
				Parts:       []string{"a", "b"},
				Blanks:      []Blank{{Infinitive: "ir", Answer: "voy", Tense: "present"}},
				Rationale:   []string{"because"},
			},
		},
	}
}

func TestHandleSentences_ReturnsJSONArray(t *testing.T) {
	defer func() { currentStory = Story{} }()
	setupTestStory()

	req := httptest.NewRequest(http.MethodGet, "/api/sentences", nil)
	w := httptest.NewRecorder()
	handleSentences(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got []Sentence
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != 42 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestHandleCheck_HappyPathAndLenientAccent(t *testing.T) {
	defer func() { currentStory = Story{} }()
	setupTestStory()
	body := strings.NewReader(`{"sentence_id":42,"answers":["VÓY"]}`) // accent and case; normalize should pass
	req := httptest.NewRequest(http.MethodPost, "/api/check", body)
	w := httptest.NewRecorder()
	handleCheck(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var cr CheckResult
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(cr.Results) != 1 || !cr.Results[0] {
		t.Fatalf("expected correct result, got %+v", cr)
	}
	if len(cr.Rationale) != 1 || cr.Rationale[0] == "" {
		t.Fatalf("expected rationale passthrough, got %+v", cr)
	}
}

func TestHandleCheck_SentenceNotFound(t *testing.T) {
	defer func() { currentStory = Story{} }()
	setupTestStory()
	body := strings.NewReader(`{"sentence_id":999,"answers":["x"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/check", body)
	w := httptest.NewRecorder()
	handleCheck(w, req)
	if w.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Result().StatusCode)
	}
}

func TestHandleCheck_AnswerCountMismatch(t *testing.T) {
	defer func() { currentStory = Story{} }()
	setupTestStory()
	body := strings.NewReader(`{"sentence_id":42,"answers":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/check", body)
	w := httptest.NewRecorder()
	handleCheck(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
}

// ------------------------
// listStoryFiles
// ------------------------

func TestListStoryFiles_FiltersJSON(t *testing.T) {
	dir := t.TempDir()
	// create files
	mustWrite := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	mustWrite("a.json")
	mustWrite("b.JSON")
	mustWrite("c.txt")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// a file in subdir should be ignored by top-level scan
	if err := os.WriteFile(filepath.Join(dir, "sub", "d.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := listStoryFiles(dir)
	if err != nil {
		t.Fatalf("listStoryFiles error: %v", err)
	}
	// Order is not guaranteed by os.ReadDir; use a set
	m := map[string]bool{}
	for _, g := range got {
		m[g] = true
	}
	if !m["a.json"] || !m["b.JSON"] {
		t.Fatalf("expected JSON files present, got %v", got)
	}
	if m["c.txt"] {
		t.Fatalf("did not expect non-json file, got %v", got)
	}
}
