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
	ID        int      `json:"id"`
	Parts     []string `json:"parts"` // len(Parts) == len(Blanks)+1
	Blanks    []Blank  `json:"blanks"`
	Rationale []string `json:"rationale"` // one entry per blank
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

var sentences []Sentence

func loadSentencesFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data []Sentence
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	sentences = data
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
	json.NewEncoder(w).Encode(sentences)
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
	for i := range sentences {
		if sentences[i].ID == req.SentenceID {
			s = &sentences[i]
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
	case "preterite":
		return "badge-pret"
	case "imperfect":
		return "badge-imp"
	case "subjunctive":
		return "badge-subj"
	case "subjunctive_imperfect":
		return "badge-subj-imp"
	case "imperative":
		return "badge-imper"
	case "conditional":
		return "badge-cond"
	case "future":
		return "badge-fut"
	default:
		return "badge-pres"
	}
}

func tenseBadgeLabel(tense string) string {
	switch tense {
	case "preterite":
		return "pretérito"
	case "imperfect":
		return "imperfecto"
	case "subjunctive":
		return "subjuntivo"
	case "subjunctive_imperfect":
		return "subjuntivo imperfecto"
	case "imperative":
		return "imperativo"
	case "conditional":
		return "condicional"
	case "future":
		return "futuro"
	default:
		return "presente"
	}
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
	if err := tmplIndex.Execute(w, data); err != nil {
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
	if err := tmplStory.Execute(w, sentences); err != nil {
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
	if err := tmplStory.Execute(w, sentences); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
