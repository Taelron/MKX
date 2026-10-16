# mkx

Make target runner TUI with k9s-style navigation. Workspace-level rules and the collaboration model live in the parent directory's `CLAUDE.md` — this file only carries mkx-specific context.

Linear is canonical. Everything below points at Linear rather than restating it; when this file and Linear disagree, Linear wins.

## Current state

**Phase 1 — minimum viable implementation.** Per @AI Workflow, Phase 1 ends with v1.0: an installable, usable artifact delivering the core value the spec promises. The phase is gated on that deliverable, not on a schedule.

**Active milestone: M3 — Loose ends** (in progress) — [MkX project in Linear](https://linear.app/taelron/project/mkx-7067f8030ab4).

This file is updated when the active milestone changes, typically once per milestone closure.

<!-- taelron:workflow:start -->
## Taelron AI Build Workflow

Every Taelron product runs the
[AI Build Workflow](https://linear.app/taelron/document/ai-build-workflow-bedca8de2907),
version 2. It is canonical; this section is a summary. Re-read it at session start,
and where this section and it disagree, it wins. Your role prompt says which steps
are yours.

### Roles

| # | Name            | Runs on                  | Model    | Role                            |
|---|-----------------|--------------------------|----------|---------------------------------|
| 1 | PO              | Human                    | —        | Product decisions, `go`, merge  |
| 2 | Systems Analyst | Claude Desktop           | Opus 5   | Design, Milestone Plan, close   |
| 3 | Orchestrator    | Claude Code, own clone   | Opus 5   | Runs the milestone              |
| 4 | Architect       | Claude Code              | Fable 5  | Issue check, Implementation Plan |
| 5 | Tester          | Claude Code              | Sonnet 5 | Acceptance tests, from the issue |
| 6 | Dev             | Claude Code              | Sonnet 5 | Code, push, draft PR, brief     |
| 7 | Reviewer        | Claude Code              | Opus 5   | Fresh-context review of a diff  |
| 8 | Verifier        | Claude Code              | Sonnet 5 | Drives the built binary         |

2. Owns design, ADRs, the Milestone Plan and its issues, amendments that change
   what the product promises, and the milestone close. Writes Linear.
3. Runs outside the product repository, from Linear and a read-only clone. Starts
   each role, reviews the Implementation Plan, posts the APPROVED PLAN, settles
   technical disputes and conflicts, routes open items with the PO, writes the
   milestone report. Never runs the Architect's model.
8. Fixtures per fleet type. Reports; never fixes.

### Rules for every phase

- **The PO links the two sides only at the boundary of a milestone.** One paste in
  (Systems Analyst → Orchestrator), one paste out (Orchestrator → Systems Analyst).
  Inside a milestone the Orchestrator carries every handoff, and Claude Code
  sessions message each other directly with `SendMessage`, addressed by session
  name (`<product>-<role>`).
- **The PO answers in one pane, the Orchestrator's.** Every question, the plan
  approval, the merge and the routing reach the PO there; the PO types the answer.
- **Promises and design go to the Systems Analyst.** An amendment that adds, removes
  or weakens an acceptance criterion, or a question about a design decision, leaves
  the milestone through the PO: one paste out, one paste back.
- **A handover across the boundary ends with a prompt.** Details go to Linear first,
  then a short, ready-to-paste prompt: what changed, and where to read it.
- **Decisions for the PO are numbered, each with a recommendation and why.** The PO
  answers by number, or `ok go` to apply every recommendation.
- **The PO is asked product questions only:** what the user sees, what the product
  promises, scope, licence, money, anything irreversible. Anything derivable from a
  baseline, a document or the machine is decided by the session that meets it, and
  recorded where the next reader finds it. A disagreement on a fact is settled by
  measuring.
- **If it changes the issue or the plan, it goes on the issue.** The terminal is for
  conversation; Linear is the record.
- **Comments post directly.** Issue checks, decisions, plans, reviews, the APPROVED
  PLAN, briefs, routes, the milestone report, and an amendment that corrects a fact
  or names an interface. A change to what the product promises, a product
  document, a baseline, or a milestone's scope or exit criterion is shown to the PO
  first, by the Systems Analyst.
- **Two rounds, then escalate.** Every loop stops after two rounds without agreement
  and goes to the PO, through the Orchestrator.

### Phase rules

- **A · Planning** (PO, Systems Analyst). One issue per milestone unless a named
  reason forces a split. The plan says what, never how. Nothing is written to Linear
  before the PO validates the plan. The milestone is locked once created. The issue
  names the interface its tests rely on, the repository, the issue branch and the
  source of the toolchain versions.
- **B · Implementation planning** (Orchestrator, Architect). The Architect session
  is fresh for each milestone, when the PO starts the layout; between issues the
  Orchestrator's start prompt names the new issue and its sources. The issue check is posted before any amendment it argues for;
  the decisions before the plan. The Architect asks product questions and decides
  technical ones. The plan is a comment titled `Implementation Plan, revision <n>`.
  After the PO's `go`, the Orchestrator posts `APPROVED PLAN` with the plan in full.
  No code is written in this phase.
- **C · Build** (Orchestrator, Tester, Dev, Reviewer, Verifier). The Tester and Dev
  share one folder and one branch; the Verifier builds from a clean clone; the
  Orchestrator reads its own clone and writes none. The Tester writes tests from the
  issue alone and commits the red run, taken from a clean copy of its commit. Dev
  never modifies an acceptance test: a dispute goes to the Orchestrator. The Reviewer
  gets the diff and the issue, never the PR. The Verifier gets the branch, the PR
  URL, the issue and the plan, never the PR body; it reports and never fixes. On
  disagreement the acceptance test wins. Nothing is pushed before the tests and the
  code; the PR is a draft carrying the verification handoff (ordered Make targets,
  purpose, expected result). The brief is a comment on the issue and gates the
  merge. The PO verifies, marks the PR ready and merges.

### Tracing: pick-up first, then the listed events

- **Pick-up rule.** The moment you receive work, from the PO or from another
  session, your first action, before reading or doing anything else, is the `R`
  event, with `--ball` set to yourself:
  `~/.local/bin/taelron-trace --product <product> --milestone <milestone> --issue <issue> --phase <phase> --event R --step <step> --from <you> --ball <you> --next '<what you do now>'`.
  This covers every handoff, including each round of a loop.
- **A started event is the pick-up for its step.** Where your step opens with a
  started event (A·T1, B·T1, C·T1), send it instead of `R`.
- **Listed events.** Send the events your role prompt lists, only once their step
  is complete. `taelron-trace --help` lists every event; it refuses any other.
  Never invent one. `--link` is optional: the PR or Linear URL the event points to.
- **The `next` rule.** `--next` is what the ball-holder does next, in a few words.

### Linear access

Claude Code reads and writes Linear with curl against
`https://api.linear.app/graphql`, never the Linear MCP. The Architect, Tester, Dev
and Verifier use the key in `LINEAR_API_KEY`; the Orchestrator reads its key from
`pass` at each call. The key never goes in a file, in argv or in shell history:
pass the header on a file descriptor, e.g. `-H @<(printf 'Authorization: %s\n' "$LINEAR_API_KEY")`.
Fetch documents in full with `document(id:)`, which accepts the URL slug; a list
response is truncated. Writes are issue comments and review-gated `spec-drift`
issues; the Orchestrator may also apply an amendment that corrects a fact or names
an interface to the issue itself. The Reviewer has no Linear access, by design.

### Session read order

1. [AI Build Workflow](https://linear.app/taelron/document/ai-build-workflow-bedca8de2907)
   — participants, models, the three phases.
2. [AI Workflow](https://linear.app/taelron/document/ai-workflow-805fb2002fce)
   — surfaces, author capability, the Linear write gate, phased delivery.
3. [Delivery Workflow](https://linear.app/taelron/document/delivery-workflow-edab9d0993e8)
   and whichever other baselines apply to the task.
4. The product documents: Domain Model, ADRs, Specification, Technical Design or
   Design Notes.
5. The issue you were given, its milestone and its Milestone Plan.

Before offering a decision, open the document that would record it.
<!-- taelron:workflow:end -->

## ADRs are the Systems Analyst's lane

Claude Code **never authors Linear documents or ADRs** (@AI Workflow). Its Linear writes are limited to issue comments and review-gated `spec-drift` issues. If implementation surfaces a conflict with an ADR, follow the drift handling protocol in @Delivery Workflow — surface it, never silently diverge, and let the Systems Analyst record the decision.

- @MkX ADR Index ([Linear](https://linear.app/taelron/document/mkx-adr-index-8558ef2aea57)) — the four accepted MkX ADRs, the `ADR-M<nnn>` numbering convention, and known future ADR topics

## Baselines that govern this repo

- @AI Workflow ([Linear](https://linear.app/taelron/document/ai-workflow-805fb2002fce)) — who owns which surface, phased delivery, verification by evidence
- @Delivery Workflow ([Linear](https://linear.app/taelron/document/delivery-workflow-edab9d0993e8)) — milestone rhythm, drift handling, session entry, the PR verification handoff
- @Hexagonal Architecture ([Linear](https://linear.app/taelron/document/hexagonal-architecture-b142001f420e)) — Go layer structure and the port/adapter contract
- @TUI Go Conventions ([Linear](https://linear.app/taelron/document/tui-go-conventions-1aca4ef63a66)) — Bubble Tea, Lipgloss, async patterns
