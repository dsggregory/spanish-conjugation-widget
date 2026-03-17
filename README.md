# Spanish Verb Exerciser

A story-based Spanish verb conjugation exercise frontend to AI-generated JSON stories.

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
- `translation` (string) — full-sentence translation in English; shown as a tooltip when you hover the sentence label ("Sentence N").
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
    "translation": "When I was little, my grandfather used to play the guitar.",
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

## Ask AI for a new story
Here's a prompt you could paste into a new AI chat. Take the resulting JSON story and save into the `./stories` directory.

> Write a Spanish verb conjugation exercise as a JSON file using this exact structure:
>
> ```json
> [
>   {
>     "id": 0,
>     "translation": "Sentence translation in english.",
>     "parts": ["Text before first blank ", " text between blanks ", " text after last blank."],
>     "blanks": [
>       {
>         "infinitive": "verb in infinitive form",
>         "answer": "correct conjugation",
>         "tense": "tense identifier string",
>         "hint": "verb (person, tense abbreviation)"
>       }
>     ],
>     "rationale": [
>       "conjugation → tense name: explanation of why this tense is used here."
>     ]
>   }
> ]
> ```
>
> Rules:
> - `parts` always has exactly one more element than `blanks` — they interleave: part[0], blank[0], part[1], blank[1], etc.
> - `tense` should be one of: `present`, `preterite`, `imperfect`, `present_perfect`, `past_perfect`, `future`, `future_perfect`, `conditional`, `conditional_perfect`, `subjunctive_present`, `subjunctive_imperfect`
> - `rationale` has one entry per blank, explaining the grammar rule that determines the tense choice
> - Answer matching should be lenient — accept answers with or without accent marks
>
> Write a short story of 5–6 sentences on the topic of [YOUR TOPIC HERE] using the tenses [LIST TENSES HERE]. Make the tense choices pedagogically deliberate — pair contrasting tenses within sentences where possible to highlight the distinction.

Swap in your topic and target tenses at the bottom. For example:
- *"a trip to a foreign city"* using *preterite, imperfect, and present perfect*
- *"a career decision"* using *conditional, future perfect, and subjunctive*
- *"childhood memories"* using *imperfect and preterite*

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

