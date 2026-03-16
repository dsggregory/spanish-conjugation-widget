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
	tmplIndex, err = template.New("index").Funcs(fm).Parse(indexHTML)
	if err != nil {
		log.Fatal(err)
	}
	tmplStory, err = template.New("story").Funcs(fm).Parse(storyHTML)
	if err != nil {
		log.Fatal(err)
	}

	addr := ":8080"
	log.Printf("Listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Embedded HTML/JS front-end (templates)
// ---------------------------------------------------------------------------

const indexHTML = `<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>La guitarra de mi abuelo — Spanish Verb Exercise</title>
<style>
  *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: system-ui, -apple-system, sans-serif;
    background: #f5f4f0;
    color: #1a1a1a;
    padding: 2rem 1rem;
    line-height: 1.6;
  }
  .container { max-width: 780px; margin: 0 auto; }
  h1 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.25rem; }
  .subtitle { font-size: 0.875rem; color: #666; margin-bottom: 1.5rem; }

  /* Legend */
  .legend { display: flex; gap: 1rem; flex-wrap: wrap; margin-bottom: 1.5rem; }
  .badge {
    display: inline-block; font-size: 0.7rem; font-weight: 600;
    padding: 2px 10px; border-radius: 99px; letter-spacing: 0.03em;
  }
  .badge-pret { background: #faeeda; color: #854f0b; }
  .badge-imp  { background: #e1f5ee; color: #0f6e56; }
  .badge-pres { background: #e6f1fb; color: #185fa5; }
	.badge-impr { background: #f0ede6; color: #7e600b; }
	.badge-perf { background: #eaf3de; color: #3b6d11; }
	.badge-subj { background: #e6f1fb; color: #185fa5; }
	.badge-subj-imp { background: #f0ede6; color: #7e600b; }
	.badge-impr { background: #f0ede6; color: #7e600b; }
  .leg-item { display: flex; align-items: center; gap: 6px; font-size: 0.8rem; color: #555; }

  /* Cards */
  .card {
    background: #fff;
    border: 1px solid #e2e0d8;
    border-radius: 12px;
    padding: 1rem 1.25rem;
    margin-bottom: 12px;
  }
  .card-num { font-size: 0.7rem; color: #999; text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 8px; }
  .sentence  { font-size: 1rem; line-height: 2.4; }

  /* Blank groups */
  .blank-group {
    display: inline-flex; flex-direction: column;
    align-items: center; vertical-align: middle;
    margin: 0 4px;
  }
  .blank-group input {
    border: none; border-bottom: 1.5px solid #aaa;
    background: transparent; font-size: 1rem;
    width: 120px; text-align: center; outline: none;
    padding: 1px 4px; font-family: inherit; color: #1a1a1a;
    transition: border-color 0.15s;
  }
  .blank-group input:focus { border-bottom-color: #378add; }
  .blank-group input.correct  { border-bottom-color: #3b6d11; color: #3b6d11; }
  .blank-group input.incorrect{ border-bottom-color: #e24b4a; color: #a32d2d; }
  .blank-group input.revealed { border-bottom-color: #378add; color: #185fa5; background: #e6f1fb; border-radius: 3px; }
  .blank-hint { font-size: 0.68rem; color: #999; font-style: italic; margin-top: 2px; }

  /* Rationale */
  .rationale {
    display: none; margin-top: 10px; font-size: 0.82rem; line-height: 1.55;
    padding: 8px 12px; border-radius: 8px;
  }
  .rationale.show { display: block; }
  .rationale.correct   { background: #eaf3de; color: #3b6d11; border-left: 3px solid #639922; }
  .rationale.incorrect { background: #fcebeb; color: #a32d2d; border-left: 3px solid #e24b4a; }
  .rationale.revealed  { background: #e6f1fb; color: #185fa5; border-left: 3px solid #378add; }

  /* Buttons */
  .controls { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 1.5rem; }
  .controls button {
    cursor: pointer; font-family: inherit; font-size: 0.875rem;
    background: #fff; border: 1px solid #ccc; border-radius: 8px;
    padding: 8px 20px; color: #333; transition: background 0.15s;
  }
  .controls button:hover { background: #f0ede6; }
  #score { font-size: 0.875rem; color: #555; margin-top: 12px; min-height: 1.2em; }

  /* Progress bar */
  .progress-wrap { background: #e8e5de; border-radius: 99px; height: 6px; margin-bottom: 1.5rem; overflow: hidden; }
  .progress-bar  { background: #3b6d11; height: 100%; border-radius: 99px; width: 0%; transition: width 0.4s ease; }
</style>
<script src="https://unpkg.com/htmx.org@1.9.12"></script>
</head>
<body>
<div class="container">
  <h1>La guitarra de mi abuelo</h1>
  <p class="subtitle">Type the correct conjugation for each infinitive, then check your answers.</p>

  <div style="margin-bottom: 0.75rem; font-size: 0.9rem; color: #444;">
    <input type="checkbox" id="toggle-badges" checked>
    <label for="toggle-badges">Hide tense badges</label>
  </div>

  <div style="margin-bottom: 1rem; font-size: 0.9rem; color: #444; display: flex; gap: 10px; align-items: center;">
    <label for="story-file" style="min-width: 110px;">Story file:</label>
    <select id="story-file" name="file"
            hx-get="/load-story" hx-trigger="change" hx-target="#story" hx-swap="innerHTML">
      <option value="">-- Select a story --</option>
      {{- range .StoryFiles }}
      <option value="{{ . }}">{{ . }}</option>
      {{- end }}
    </select>
  </div>

  <div class="legend">
    <div class="leg-item"><span class="badge badge-pret">pretérito</span> completed past action</div>
    <div class="leg-item"><span class="badge badge-imp">imperfecto</span> ongoing / habitual / descriptive</div>
    <div class="leg-item"><span class="badge badge-pres">presente</span> present tense</div>
	<div class="leg-item"><span class="badge badge-impr">imperativo</span> command action</div>
	<div class="leg-item"><span class="badge badge-perf">perfecto</span> completed future action</div>
	<div class="leg-item"><span class="badge badge-subj">subjuntivo</span> desires, wishes</div>
	<div class="leg-item"><span class="badge badge-subj-imp">subjuntivo imperfecto</span> past desires, wishes</div>
  </div>

  <div class="progress-wrap"><div class="progress-bar" id="progress"></div></div>

  <div id="story"><div class="card"><div class="sentence" style="color:#666;">No story loaded. Choose a JSON file from the dropdown above.</div></div></div>

  <div class="controls">
    <button onclick="checkAll()">Check answers</button>
    <button onclick="revealAll()">Show answers</button>
    <button onclick="resetAll()">Reset</button>
  </div>
  <div id="score"></div>
</div>

<script>
let sentenceData = [];

async function loadSentences() {
  const res = await fetch('/api/sentences');
  sentenceData = await res.json();
}

function badgeClass(tense) {
  if (tense === 'preterite') return 'badge-pret';
  if (tense === 'imperfect') return 'badge-imp';
  if (tense === 'subjunctive') return 'badge-subj';
  if (tense === 'imperative') return 'badge-impr';
  if (tense === 'perfect') return 'badge-perf';
  if (tense === 'subjunctive_imperfect') return 'badge-subj-imp';
  return 'badge-pres';
}

function badgeLabel(tense) {
  if (tense === 'preterite') return 'pretérito';
  if (tense === 'imperfect') return 'imperfecto';
  return tense;
}

// HTMX hook to update progress when story content is loaded
document.body.addEventListener('htmx:afterSettle', function(evt) {
  if (evt && evt.target && evt.target.id === 'story') {
    // Reload sentence metadata first, then wire inputs and update UI
    loadSentences().then(() => {
      document.querySelectorAll('.blank-group input').forEach(inp => {
        inp.addEventListener('input', updateProgress);
      });
      updateProgress();
      if (typeof applyBadgeVisibility === 'function') {
        applyBadgeVisibility();
      }
    }).catch(() => {
      // Even if metadata fails, try to wire inputs to avoid breaking UX
      document.querySelectorAll('.blank-group input').forEach(inp => {
        inp.addEventListener('input', updateProgress);
      });
      updateProgress();
      if (typeof applyBadgeVisibility === 'function') {
        applyBadgeVisibility();
      }
    });
  }
});

function updateProgress() {
  let filled = 0, total = 0;
  sentenceData.forEach(s => {
    s.blanks.forEach((_, bi) => {
      total++;
      const inp = document.getElementById('inp-' + s.id + '-' + bi);
      if (inp && inp.value.trim() !== '') filled++;
    });
  });
  const pct = total ? Math.round((filled / total) * 100) : 0;
  document.getElementById('progress').style.width = pct + '%';
}

async function checkAll() {
  let correct = 0, total = 0;

  for (const s of sentenceData) {
    // Collect answers for this sentence
    const answers = s.blanks.map((_, bi) => {
      const inp = document.getElementById('inp-' + s.id + '-' + bi);
      return inp ? inp.value : '';
    });

    // Skip if all already revealed
    const allRevealed = s.blanks.every((_, bi) =>
      document.getElementById('inp-' + s.id + '-' + bi)?.classList.contains('revealed')
    );
    if (allRevealed) continue;

    const res = await fetch('/api/check', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({sentence_id: s.id, answers})
    });
    const data = await res.json();

    s.blanks.forEach((_, bi) => {
      const inp = document.getElementById('inp-' + s.id + '-' + bi);
      const rat = document.getElementById('rat-' + s.id + '-' + bi);
      if (!inp || inp.classList.contains('revealed')) return;

      total++;
      if (data.results[bi]) {
        inp.className = 'correct';
        rat.className = 'rationale show correct';
        correct++;
      } else {
        inp.className = 'incorrect';
        rat.className = 'rationale show incorrect';
      }
      rat.textContent = data.rationale[bi];
    });
  }

  document.getElementById('score').textContent = 'Score: ' + correct + ' / ' + total + ' correct';
}

async function revealAll() {
  for (const s of sentenceData) {
    const answers = s.blanks.map(b => b.answer);
    const res = await fetch('/api/check', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({sentence_id: s.id, answers})
    });
    const data = await res.json();

    s.blanks.forEach((b, bi) => {
      const inp = document.getElementById('inp-' + s.id + '-' + bi);
      const rat = document.getElementById('rat-' + s.id + '-' + bi);
      if (!inp) return;
      inp.value = b.answer;
      inp.className = 'revealed';
      rat.className = 'rationale show revealed';
      rat.textContent = data.rationale[bi];
    });
  }
  document.getElementById('score').textContent = '';
}

function resetAll() {
  sentenceData.forEach(s => {
    s.blanks.forEach((_, bi) => {
      const inp = document.getElementById('inp-' + s.id + '-' + bi);
      const rat = document.getElementById('rat-' + s.id + '-' + bi);
      if (!inp) return;
      inp.value = '';
      inp.className = '';
      rat.className = 'rationale';
      rat.textContent = '';
    });
  });
  document.getElementById('score').textContent = '';
  updateProgress();
}

// No story is loaded initially; sentence metadata will load after a story is selected.

// Checkbox-driven hide/show of badges via CSS
const badgeToggler = document.getElementById('toggle-badges');
function applyBadgeVisibility() {
  const checked = badgeToggler.checked;
  document.querySelectorAll('.badge').forEach(b => {
    b.style.display = checked ? 'none' : 'inline-block';
  });
}
badgeToggler.addEventListener('change', applyBadgeVisibility);
document.addEventListener('DOMContentLoaded', applyBadgeVisibility);
</script>
</body>
</html>
`

// Story partial (server-rendered with go-template, loaded by HTMX)
const storyHTML = `{{/* expects []Sentence */}}
{{ range . }}
  {{ $s := . }}
  <div class="card" id="card-{{ $s.ID }}">
    <div class="card-num">Sentence {{ add $s.ID 1 }}</div>
    <div class="sentence">
      {{ $sid := $s.ID }}
      {{ $parts := $s.Parts }}
      {{ range $i, $p := $parts }}
        {{ $p }}
        {{ if lt $i (len $s.Blanks) }}
          {{ $b := index $s.Blanks $i }}
          <span class="blank-group">
            <input type="text" id="inp-{{ $sid }}-{{ $i }}" autocomplete="off" autocorrect="off" spellcheck="false">
            <span class="blank-hint">{{ $b.Infinitive }} → ?</span>
            <span class="badge {{ badgeClass $b.Tense }}">{{ badgeLabel $b.Tense }}</span>
          </span>
        {{ end }}
      {{ end }}
    </div>
    {{ range $i, $_ := $s.Blanks }}
      <div class="rationale" id="rat-{{ $sid }}-{{ $i }}"></div>
    {{ end }}
  </div>
{{ end }}`
