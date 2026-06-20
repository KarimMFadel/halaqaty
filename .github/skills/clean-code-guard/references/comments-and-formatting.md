# Comments and Formatting — Clean Code Chapters 4 and 5

Source: Robert C. Martin, *Clean Code*. Summaries: Vivek Khatri Ch. 4, Vivek Khatri Ch. 5, LinkedIn summary of Ch. 4.

## Contents

- Comments
  - C1. Acceptable comments
  - C2. Banned comments
  - C3. Docstring discipline
- Formatting
  - Fmt1. Vertical openness separates concepts
  - Fmt2. Vertical density implies association
  - Fmt3. Vertical distance
  - Fmt4. Horizontal density
  - Fmt5. Match the file you're editing
- Self-check for comments and formatting

## Comments

The foundational rule: **"Don't comment bad code — rewrite it."** Comments are failures to express intent in code. Every comment is a candidate for rename or extract.

### C1. Acceptable comments

A short list of comments that earn their keep:

- **Legal headers** — license boilerplate, copyright.
- **Intent** — explaining *why* a decision was made when the choice is non-obvious. Example: `// Use exponential backoff to avoid hammering the rate limiter during retries.`
- **Warnings of consequences** — `// This function is called during transaction commit; do not raise.`
- **TODOs** — sparingly, with a tracking ticket reference. `// TODO(JIRA-1234): switch to streaming once API supports it.`
- **Public API documentation** — docstrings that document *contract* (preconditions, postconditions, raises), not body.
- **Amplification** — calling attention to something non-obvious. `// The +1 accounts for the inclusive end of the range; see RFC §3.2.`

### C2. Banned comments

Delete on sight:

- **Restating-code comments.** `// increment counter by one` above `counter += 1`. The comment adds zero signal and creates two things to maintain.
- **Noise comments.** `# default constructor`, `# getter`, `# returns the day of month`.
- **Banner comments.** `# ====== USER FUNCTIONS ======`. Use a class or module split instead.
- **Closing-brace comments.** `}  // end of for loop`. If you need this to follow the flow, the function is too long.
- **Attributions and journal comments.** `# Updated by Bob on 2023-04-01 to fix bug #42`. Version control records this.
- **Commented-out code.** Delete it. If you need it back, git has it. Commented blocks are toxic — readers don't know whether to trust them.
- **`Step 1` / `Step 2` scaffolding.** Common LLM artifact. Each step should be a function call with a name; the names provide the structure.

### C3. Docstring discipline

A documentation comment that paraphrases the function signature is noise:

**Bad:**
```go
// GetCircle gets the circle by ID.
func GetCircle(id uuid.UUID) (*Circle, error) { ... }
```

**Good:**
```go
// GetCircle returns the circle for the given ID.
// Returns ErrCircleNotFound if no circle with that ID exists.
func GetCircle(id uuid.UUID) (*Circle, error) { ... }
```

A documentation comment earns its keep when it documents contract: what may be passed, what may be returned, what errors are raised, and any non-obvious side effects.

---

## Formatting

### Fmt1. Vertical openness separates concepts

Blank lines between concepts. No blank lines inside a tightly-coupled block. The eye uses blank lines as boundaries.

### Fmt2. Vertical density implies association

Code that belongs together should sit together. Variable declared 30 lines from its use is a smell.

### Fmt3. Vertical distance

- **Variables declared close to use.** Not at the top of the function "C-style."
- **Caller above callee.** Top-down reading: high-level function first, then the helpers it calls.
- **Conceptually related functions adjacent.** If `parseInvoice` and `validateInvoice` are siblings, put them next to each other.

### Fmt4. Horizontal density

- Spaces around assignment and comparison operators.
- No space between function name and parenthesis.
- **Go**: line length ≤100 characters; `gofmt` handles formatting automatically — run it.
- **Dart**: line length ≤80 characters by default; `dart format` handles it — run it.

### Fmt5. Match the file you're editing

The most common cross-cutting violation: introducing a new style in a file that already had one.

- **Go**: imports follow the three-group convention (stdlib / external / internal). Use `goimports`. If the file uses a particular error-wrapping style, match it.
- **Dart**: if the file uses `riverpod` for state management, don't introduce a different pattern in the same file.

Team rules override personal preference. Read the file, then write.

---

## Self-check for comments and formatting

Before you ship code:

1. Walk every comment you added. For each, ask: does it explain *why*? If it explains *what*, delete it.
2. Walk every documentation comment you added. Is it paraphrasing the signature? Delete the paraphrase; keep only contract documentation. (Exception: Go exported symbols require a doc comment.)
3. Any commented-out code? Delete it.
4. Any `Step 1`, `Step 2`, `First, ...`, or `Then, ...` scaffolding comments? Delete.
5. Are variables declared near their use, not at the top?
6. Does the casing, quoting, and import order match the file's existing style?
7. Did you run `gofmt` / `dart format`?
