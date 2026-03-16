# La guitarra de mi abuelo — Spanish Verb Exercise

A self-contained Go web service. No external framework, one optional dependency
(`golang.org/x/text`) for accent-insensitive answer matching.

## Quick start

1) Prepare a stories directory and put one or more JSON stories inside it:

```bash
mkdir -p stories
# Option A: create your own JSON (see format below)
# Option B: use the provided sample
cp sentences.json stories/sample.json
```

2) Run the server:

```bash
go mod tidy
go run main.go
# Open http://localhost:8080
```

3) In the web UI, choose a story file from the “Story file” dropdown. By design, no
story is loaded on first run; selecting a file loads and renders it.

## Loading stories from local files

- The app looks for JSON files under the local `./stories` directory.
- Use the “Story file” dropdown in the page header to load a story. The content is
  rendered server-side and injected dynamically; you can switch stories at any time.

Security note: only files inside `./stories` with the `.json` extension are accepted.

## Story JSON format

Each story JSON is an array of sentence objects. For each sentence:

- `id` (number) — unique identifier (can be 0-based or any integers, must be stable).
- `parts` (array of strings) — textual fragments around blanks; its length must be
  exactly `len(blanks) + 1`.
- `blanks` (array) — each blank has:
  - `infinitive` (string)
  - `answer` (string) — the correct conjugated form
  - `tense` (string) — one of `"preterite" | "imperfect" | "present"`
  - `hint` (string) — short hint displayed under the input
- `rationale` (array of strings) — one explanation per blank, same length as `blanks`.

Example (single sentence):

```json
[
  {
    "id": 0,
    "parts": ["Cuando ", " pequeño, mi abuelo ", " tocar la guitarra."],
    "blanks": [
      {"infinitive": "ser", "answer": "era", "tense": "imperfect", "hint": "ser (yo, imperf.)"},
      {"infinitive": "soler", "answer": "solía", "tense": "imperfect", "hint": "soler (él, imperf.)"}
    ],
    "rationale": [
      "era → imperfecto: background state.",
      "solía → imperfecto: habitual past action."
    ]
  }
]
```

## API

| Method | Path | Description |
|--------|------|-------------|
| GET  | `/`              | Serves the HTML/JS front-end |
| GET  | `/api/sentences` | Returns all sentence objects as JSON (for the currently loaded story) |
| POST | `/api/check`     | Checks answers for one sentence; returns correctness + rationale |

Notes:
- Until you load a story, `/api/sentences` returns an empty array `[]`.
- The front-end uses these endpoints to check answers and compute progress.

### POST /api/check — request body

```json
{
  "sentence_id": 4,
  "answers": ["estaba", "toqué"]
}
```

### POST /api/check — response

```json
{
  "results":   [true, true],
  "rationale": [
    "estaba → imperfecto: describes an emotional state acting as background context.",
    "toqué → pretérito: playing those notes was a single completed action."
  ]
}
```

## Answer matching

Answers are normalised before comparison:
- Lowercase
- Accent marks stripped (so "toque" matches "toqué")
- Leading/trailing whitespace collapsed

This means learners are not penalised for missing accent marks.

## Structure

```
main.go        — service: handlers + embedded HTML templates
sentences.json — sample story you can copy into ./stories
stories/       — place your story JSON files here (not tracked by default)
go.mod
README.md
```
