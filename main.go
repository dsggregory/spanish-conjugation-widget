package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ---------------------------------------------------------------------------
// Data model
// ---------------------------------------------------------------------------

type Blank struct {
	Infinitive string `json:"infinitive"`
	Answer     string `json:"answer"`
	Tense      string `json:"tense"` // "preterite" | "imperfect" | "present"
	Hint       string `json:"hint"`
}

type Sentence struct {
	ID          int      `json:"id"`
	Translation string   `json:"translation"` // translation of the full sentence in english
	Parts       []string `json:"parts"`       // len(Parts) == len(Blanks)+1
	Blanks      []Blank  `json:"blanks"`
	Rationale   []string `json:"rationale"` // one entry per blank
}

// Story is a container for a set of sentences, plus an optional overview
// that describes gotchas or idiosyncrasies in the story.
type Note struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

type Overview struct {
	Title       string   `json:"title"`
	Topic       string   `json:"topic"`
	Tenses      []string `json:"tenses"`
	Translation string   `json:"translation"`
	Notes       []Note   `json:"notes"`
}

type Story struct {
	Overview  *Overview  `json:"overview"`
	Sentences []Sentence `json:"sentences"`
}

// CheckRequest is the JSON body sent by the browser on "Check answers".
type CheckRequest struct {
	SentenceID int      `json:"sentence_id"`
	Answers    []string `json:"answers"`
}

// CheckResult is the JSON response.
type CheckResult struct {
	Results   []bool   `json:"results"` // true = correct per blank
	Rationale []string `json:"rationale"`
}

// ---------------------------------------------------------------------------
// Exercise data (loaded from JSON file)
// ---------------------------------------------------------------------------

var currentStory Story

func loadSentencesFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// First try to parse as the current Story container format (with structured Overview).
	var obj Story
	if err := json.Unmarshal(b, &obj); err == nil {
		if obj.Sentences != nil { // accept if sentences array present (even if empty)
			currentStory = obj
			return nil
		}
	}
	// Backward compatibility: Story v1 where overview was a simple string.
	type storyV1 struct {
		Overview  string     `json:"overview"`
		Sentences []Sentence `json:"sentences"`
	}
	var objV1 storyV1
	if err := json.Unmarshal(b, &objV1); err == nil {
		if objV1.Sentences != nil || objV1.Overview != "" {
			var ov *Overview
			if strings.TrimSpace(objV1.Overview) != "" {
				ov = &Overview{Notes: []Note{{Heading: "Overview", Body: objV1.Overview}}}
			}
			currentStory = Story{Overview: ov, Sentences: objV1.Sentences}
			return nil
		}
	}
	// Backward compatibility: parse legacy format (array of sentences at root).
	var data []Sentence
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	currentStory = Story{Overview: nil, Sentences: data}
	return nil
}

// listStoryFiles returns base filenames of JSON stories under ./stories.
func listStoryFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// If directory doesn't exist, treat as empty list.
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".json") {
			out = append(out, name)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// normalize strips accents, lowercases, and collapses whitespace for lenient
// answer comparison.
func normalize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	// Decompose unicode, drop combining marks (accents), recompose.
	t := norm.NFD.String(s)
	var b strings.Builder
	for _, r := range t {
		if unicode.Is(unicode.Mn, r) { // Mn = non-spacing marks
			continue
		}
		b.WriteRune(r)
	}
	result := b.String()
	// Collapse interior whitespace.
	fields := strings.Fields(result)
	return strings.Join(fields, " ")
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleSentences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentStory.Sentences)
}

func handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Find the sentence.
	var s *Sentence
	for i := range currentStory.Sentences {
		if currentStory.Sentences[i].ID == req.SentenceID {
			s = &currentStory.Sentences[i]
			break
		}
	}
	if s == nil {
		http.Error(w, "sentence not found", http.StatusNotFound)
		return
	}
	if len(req.Answers) != len(s.Blanks) {
		http.Error(w, fmt.Sprintf("expected %d answers, got %d", len(s.Blanks), len(req.Answers)), http.StatusBadRequest)
		return
	}

	results := make([]bool, len(s.Blanks))
	for i, blank := range s.Blanks {
		results[i] = normalize(req.Answers[i]) == normalize(blank.Answer)
	}

	resp := CheckResult{
		Results:   results,
		Rationale: s.Rationale,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Templates
var (
	tmplIndex *template.Template
	tmplStory *template.Template
)

func tenseBadgeClass(tense string) string {
	switch tense {
	case "present":
		return "badge-pres"
	case "present_perfect":
		return "badge-pres-perf"
	case "past_perfect":
		return "badge-past-perf"
	case "preterite":
		return "badge-pret"
	case "imperfect":
		return "badge-imp"
	case "subjunctive_present":
		return "badge-subj"
	case "subjunctive_imperfect":
		return "badge-subj-imp"
	case "imperative":
		return "badge-impr"
	case "conditional":
		return "badge-cond"
	case "conditional_perfect":
		return "badge-cond-perf"
	case "future":
		return "badge-fut"
	case "future_perfect":
		return "badge-fut-perf"
	default:
		return "badge-none"
	}
}

func tenseBadgeLabel(tense string) string {
	switch tense {
	case "present":
		return "presente"
	case "present_perfect":
		return "perfecto presente"
	case "past_perfect":
		return "perfecto pasado"
	case "preterite":
		return "pretérito"
	case "imperfect":
		return "imperfecto"
	case "subjunctive_present":
		return "subjuntivo"
	case "subjunctive_imperfect":
		return "subjuntivo imperfecto"
	case "imperative":
		return "imperativo"
	case "conditional":
		return "condicional"
	case "conditional_perfect":
		return "perfecto condicional"
	case "future":
		return "futuro"
	case "future_perfect":
		return "perfecto futuro"
	default:
		return "?" + tense + "?"
	}
}

// tenseLegendDesc provides a short human-readable description for each tense,
// used in the legend rendered with the story.
func tenseLegendDesc(tense string) string {
	switch tense {
	case "preterite":
		return "completed past action"
	case "imperfect":
		return "ongoing / habitual / descriptive"
	case "present":
		return "present tense"
	case "present_perfect":
		return "perfect present tense; he -ado/ido"
	case "past_perfect":
		return "past perfect tense; había -ado/ido"
	case "imperative":
		return "command action"
	case "conditional":
		return "conditional"
	case "conditional_perfect":
		return "perfect conditional; habría -ado/ido"
	case "future":
		return "future tense"
	case "future_perfect":
		return "perfect future tense; habré -ado/ido"
	case "subjunctive_present":
		return "desires, wishes"
	case "subjunctive_imperfect":
		return "past desires, wishes"
	default:
		return "unkown"
	}
}

// StoryViewData is the data model for the story partial template.
type StoryViewData struct {
	Overview  *Overview
	Sentences []Sentence
	Tenses    []string
}

// collectTenses returns unique tenses present in the given sentences, in order
// of first appearance.
func collectTenses(ss []Sentence) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		for _, b := range s.Blanks {
			t := b.Tense
			if t == "" {
				t = "present" // default label mapping
			}
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	storyFiles, err := listStoryFiles("./stories")
	if err != nil {
		log.Printf("error listing stories: %v", err)
		storyFiles = []string{}
	}
	data := struct{ StoryFiles []string }{StoryFiles: storyFiles}
	if err := tmplIndex.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleStory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ov := currentStory.Overview
	if isOverviewEmpty(ov) {
		ov = nil
	}
	data := StoryViewData{Overview: ov, Sentences: currentStory.Sentences, Tenses: collectTenses(currentStory.Sentences)}
	if err := tmplStory.ExecuteTemplate(w, "story.gohtml", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleLoadStory loads a selected story JSON from ./stories and returns the rendered HTML.
func handleLoadStory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Read the selected filename from query or form values.
	filename := r.FormValue("file")
	if filename == "" {
		http.Error(w, "missing file parameter", http.StatusBadRequest)
		return
	}
	// Security: only allow base filenames without paths, .json extension, and must exist under ./stories
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		http.Error(w, "invalid file name", http.StatusBadRequest)
		return
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".json") {
		http.Error(w, "file must be .json", http.StatusBadRequest)
		return
	}
	full := filepath.Join("./stories", filepath.Base(filename))
	if err := loadSentencesFromFile(full); err != nil {
		http.Error(w, "failed to load story: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Render the story partial with the newly loaded sentences.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ov := currentStory.Overview
	if isOverviewEmpty(ov) {
		ov = nil
	}
	data := StoryViewData{Overview: ov, Sentences: currentStory.Sentences, Tenses: collectTenses(currentStory.Sentences)}
	if err := tmplStory.ExecuteTemplate(w, "story.gohtml", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// isOverviewEmpty returns true if the structured overview has no meaningful content.
func isOverviewEmpty(o *Overview) bool {
	if o == nil {
		return true
	}
	if strings.TrimSpace(o.Title) != "" {
		return false
	}
	if strings.TrimSpace(o.Topic) != "" {
		return false
	}
	if strings.TrimSpace(o.Translation) != "" {
		return false
	}
	if len(o.Tenses) > 0 {
		return false
	}
	if len(o.Notes) > 0 {
		// Consider notes meaningful if any note has non-empty heading or body
		for _, n := range o.Notes {
			if strings.TrimSpace(n.Heading) != "" || strings.TrimSpace(n.Body) != "" {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/sentences", handleSentences)
	mux.HandleFunc("/api/check", handleCheck)
	mux.HandleFunc("/sentences", handleStory)
	mux.HandleFunc("/load-story", handleLoadStory)

	// Do not auto-load a story on startup; app starts with none loaded.

	// Prepare templates
	fm := template.FuncMap{
		"badgeClass": tenseBadgeClass,
		"badgeLabel": tenseBadgeLabel,
		"legendDesc": tenseLegendDesc,
		"add":        func(a, b int) int { return a + b },
	}
	var err error
	tmplIndex, err = template.New("index").Funcs(fm).ParseFiles("index.html")
	if err != nil {
		log.Fatal(err)
	}
	tmplStory, err = template.New("story").Funcs(fm).ParseFiles("story.gohtml")
	if err != nil {
		log.Fatal(err)
	}

	addr := ":8080"
	log.Printf("Listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
