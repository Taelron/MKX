# mkx

Make target runner TUI with k9s-style navigation. Workspace-level rules and the collaboration model live in the parent directory's `CLAUDE.md` — this file only carries mkx-specific context.

Linear is canonical. Everything below points at Linear rather than restating it; when this file and Linear disagree, Linear wins.

## Current state

**Phase 1 — minimum viable implementation.** Per @AI Workflow, Phase 1 ends with v1.0: an installable, usable artifact delivering the core value the spec promises. The phase is gated on that deliverable, not on a schedule.

**Active milestone: M3 — Loose ends** (in progress) — [MkX project in Linear](https://linear.app/taelron/project/mkx-7067f8030ab4).

This file is updated when the active milestone changes, typically once per milestone closure.

## Taelron AI Build Workflow

This workspace runs the AI Build Workflow (Linear, Taelron Baselines). Read it at
session start; your role prompt says which phase and steps are yours.

- The PO is the notification. Claude Web (the PO Assistant) and Claude Code cannot
  reach each other. Write the details to Linear first, then end your turn with a
  short, ready-to-paste prompt for the other side: what changed, and where to read
  it. Never wait for the PO to ask what is next. The PO does not read Linear.
- Claude Code sessions message each other directly, without the PO.
- Decisions for the PO are numbered, each with your recommendation and why. The PO
  answers by number, or writes "ok go" to apply every recommendation.
- Ask the PO product questions only. Decide technical questions yourself and record
  them where the next reader will find them. Settle facts by measuring.
- If it changes the issue or the plan, it goes on the issue, as a comment.
- When you receive work, from the PO or by a message from another session, your
  first action, before reading or doing anything else, is a pick-up event:
  ~/.local/bin/taelron-trace with --event R --name "Picked up", --ball set to your
  role, the phase and step you are starting, and in --next what you are about to do.
- Send each trace event your role lists, when its step completes, with
  ~/.local/bin/taelron-trace (product, milestone and issue come from your
  handoff prompt). A failed send never blocks the work: say so in your handoff
  prompt and carry on.

## ADRs are Web Claude's lane

Claude Code **never authors Linear documents or ADRs** (@AI Workflow). Its Linear writes are limited to issue comments and review-gated `spec-drift` issues. If implementation surfaces a conflict with an ADR, follow the drift handling protocol in @Delivery Workflow — surface it, never silently diverge, and let Web Claude record the decision.

- @MkX ADR Index ([Linear](https://linear.app/taelron/document/mkx-adr-index-8558ef2aea57)) — the four accepted MkX ADRs, the `ADR-M<nnn>` numbering convention, and known future ADR topics

## Baselines that govern this repo

- @AI Workflow ([Linear](https://linear.app/taelron/document/ai-workflow-805fb2002fce)) — who owns which surface, phased delivery, verification by evidence
- @Delivery Workflow ([Linear](https://linear.app/taelron/document/delivery-workflow-edab9d0993e8)) — milestone rhythm, drift handling, session entry, the PR verification handoff
- @Hexagonal Architecture ([Linear](https://linear.app/taelron/document/hexagonal-architecture-b142001f420e)) — Go layer structure and the port/adapter contract
- @TUI Go Conventions ([Linear](https://linear.app/taelron/document/tui-go-conventions-1aca4ef63a66)) — Bubble Tea, Lipgloss, async patterns
