# Nami

An open-source local app that reads a codebase and draws it as a dependency map.
Everything on screen comes from really parsing the code. The AI explains what
the analyzers found, it never decides what's there.

Run it directly inside a repository:

```bash
nami map .
```

Why any of these rules exist is in `local project notes`. This file is the
rules themselves.

## Stack

Go for the core application, CLI, API and MCP server. SQLite for persisted
analysis and graph data.

The web interface uses Next.js, React, TypeScript strict, React Flow and dagre.
Tailwind for styling.

Language analysis is implemented separately per language behind a shared
analyzer contract. Start with Go, then Python, then TypeScript/JavaScript.

AI is optional and bring-your-own-key. No AI provider is required for Nami's
core analysis or graph features.

CI runs from the start. Deployment should remain possible later without adding
deployment infrastructure before it is needed.

If you're not certain about an API, read the current package or official
documentation rather than going from memory.

## How we work

Spec driven. Nothing gets built without a spec.

- `local project notes` — what this app is and every decision behind it. Read
  the part you need. Don't ask me to paste it.
- `local phase specs/phase-NN.md` — one per phase, written just before it starts.
  Behaviour and an acceptance check, never filenames.
- This file — always true, read on every prompt.

**Starting a phase.** Read this file and that phase's spec. Build what the spec
asks and stop.

**Dropped into a fresh context and don't know where we are?** Look at which
files exist in `local phase specs/`, then the git log. The last commit is the last
phase that passed. Tell me what you've worked out before building on it.

**The acceptance check is mine to run, not yours.** Anything requiring visual
judgement or browser interaction is mine. Don't automate browser acceptance
checks unless a spec explicitly asks for it.

**What you check before saying a phase is done:** tests, lint, build and anything
else verifiable from the terminal in a few seconds. Running a command and
reading its output counts.

**When a phase comes out wrong, I reset rather than patch.** Prompting on top of
wrong code three times leaves code nobody understands, including you. So if a
spec is ambiguous, say so _before_ you build.

## How to talk to me

Short. If a sentence isn't telling me something I need, cut it.

**Ask with an answer attached.** One specific question, and say which way you'd
go and why. "A or B, I'd take B because it keeps the analyzer standalone" is
answerable in two seconds. An open question isn't. Never pick a direction
silently, never build both.

**When you need something only I can give you** — a key, token or local config
value — say exactly what and exactly where, then stop.

**No walls of text.** Don't summarise every file you touched or restate the plan
back to me. When a phase is done, say what it does and what the check should
show, in a few lines.

**Plain English.** If you're reaching for a bulleted breakdown of something
that's one sentence, it's one sentence.

**Say when something didn't work.** A failure worked around quietly costs me an
hour later.

## How the code is laid out

You pick the file structure. These are about behaviour.

- **The scanner and analyzers can't import the web framework or UI code.**
  Repository in, graph facts out, runnable without the web app.
- **Language-specific parsing stays inside language analyzers.** Go, Python and
  TypeScript/JavaScript may use different parsing and resolution strategies, but
  they output the same graph format.
- **A repository may contain multiple languages.** Language detection happens per
  file or source group, not once for the entire repository.
- **The graph supports multiple levels.** A node may represent a directory, file,
  function or another supported source entity. The UI may fold lower levels so
  large repositories remain readable.
- **Graph calculations are deterministic.** Queries and impact analysis operate
  on graph data, not model guesses.
- **The graph builder consumes analyzer output.** Interfaces do not parse source
  themselves.
- **SQLite stores persisted scans and graph data.** Storage details stay outside
  analysis logic.
- **The web app talks to Nami through the API.** Source analysis does not happen
  inside React components or browser code.
- **CLI, Web and MCP use the same underlying graph behaviour.** Don't implement
  three versions of dependency traversal.
- **AI sits above the graph/query layer.** It retrieves facts and source, then
  explains them. It does not create graph structure.

## Analysis flow

The intended flow is:

```text
repository
    ↓
scanner
    ↓
language detection
    ↓
language analyzers
    ↓
graph fragments
    ↓
graph builder
    ↓
normalized graph
    ↓
query / impact / coverage
    ↓
CLI / Web / MCP
```

Each analyzer may work differently internally, but all analyzers return graph
facts using Nami's shared node and edge model.

Adding another language should primarily mean adding another analyzer, not
rewriting the graph, query, impact or interface layers.

## Coverage

Nami must say what it understood and what it didn't.

Coverage includes things such as files discovered, files analyzed, files
skipped, relationships resolved and relationships that could not be resolved.

If one analyzer or file fails, preserve valid results from the rest of the
repository when possible and report the failure clearly.

A clean-looking map must never silently imply complete analysis when analysis
was incomplete.

## Impact

Impact analysis shows what known graph nodes depend on the thing being changed.

It reports potential impact. It does not claim that dependent code will
definitely break.

The impact engine returns data. The web interface decides how to visualize it.

## AI

AI is optional.

Users may configure their own supported provider and API key.

The preferred flow is:

```text
question
    ↓
graph queries
    ↓
relevant graph facts
    ↓
relevant source
    ↓
AI explanation
```

AI may explain, summarize and help locate relevant code.

It may never add nodes or edges because two things appear related.

## Conventions

Go code should follow normal Go conventions and remain explicit rather than
clever.

TypeScript in the web application is strict. No `any` without discussing why.

Comment the decisions, not the syntax.

One obvious way to do something beats a configurable one.

The UI is a dense developer tool. Small type, tight spacing, monospace for file
paths. Colour means something or isn't there. Nothing moves unless it was
clicked.

Directories, files and lower-level source nodes may be folded and expanded so
the graph remains usable on large repositories.

The web interface is a view over Nami's analysis. It is not where Nami decides
what the codebase contains.

## CI and future deployment

CI exists from the beginning.

At minimum, it should eventually verify whatever the current repository can
verify automatically: tests, lint, builds and deterministic fixture analysis.

Nami should be structured so it can be distributed or deployed later without
rewriting the analysis core.

Do not introduce Docker, queues, workers, orchestration or hosted infrastructure
until a real requirement needs them.

Deployment-ready boundaries matter. Deployment-first complexity does not.

## Things not to do

Breaking one of these is worse than not finishing.

- **Never decide that two nodes are connected without deterministic evidence.**
  An edge exists because an analyzer or deterministic adapter established it.
  Unresolved gets reported with a reason, never guessed.
- **Never invent something to fill a gap.** Skipped file? Say so and count it.
  Relationship unresolved? Report it. Absent beats approximate.
- **Don't pretend a language is supported because its file extension can be
  detected.**
- **Don't put language-specific analysis into the graph, CLI, Web or MCP layers.**
- **Don't let the Web UI rebuild or reinterpret the source graph.**
- **Don't let AI create source relationships.**
- **Don't install a package without asking.** Name it, say what for, wait.
- **Don't build ahead of the current phase.** No scaffolding for what's coming.
- **Don't grade the code.** No scores, ratings, severity or "issues found". This
  explains and navigates a codebase; it doesn't review one.
- **Don't leave the build broken.** Tell me about a failure instead of working
  around it.
- **Don't weaken a check to make it pass.** A check that can't run has to fail
  loudly.
