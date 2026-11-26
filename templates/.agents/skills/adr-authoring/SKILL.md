---
name: adr-authoring
description: Use when a user asks to create, assess, review, index, or supersede an architecture decision record, or when a consequential technical decision needs durable rationale.
---

# ADR authoring

Use an architecture decision record (ADR) to preserve why a consequential technical choice was made and what follows from it. An ADR is not a task log or a place to turn every product preference into architecture.

## Before drafting

1. Read the project's agent instructions, `workspace.json` when present, its decision log, ADR index, template, and a few recent ADRs. Follow existing location, numbering, status, approval, and amendment conventions. Do not move or renumber existing records.
2. Record a decision when it affects system structure, interfaces, quality attributes, long-term maintenance, or is costly to reverse. Keep routine implementation details and product choices in their established artifacts.
3. Check the active workflow's `readPaths`, `writePaths`, allowed skills, and commands. If a separate ADR path is not permitted, keep the proposal in an allowed decision artifact or identify the exact path rule that must change. Do not widen the workspace contract yourself.
4. Separate decided choices from alternatives still under study, experiments, and open questions. An unresolved spike is not an accepted decision.

## Write the record

- Keep one consequential decision per ADR. Reuse the local template. If none exists, use a lightweight Nygard structure: **Context**, **Decision**, and **Consequences**; add alternatives when real trade-offs were considered.
- Follow local file naming. If the project has no ADR convention and the user asked for a new log, use `docs/adr/NNNN-short-action-name.md` with the next unused number and maintain a small index.
- State the scope and status (`proposed`, `accepted`, `rejected`, `deprecated`, or `superseded by <link>`). A user's explicit approval can establish acceptance within the scope they approved; do not imply broader organizational approval.
- Distinguish the date the record is written from the date a decision was made. Include only dates supported by the available context. Do not invent an approver, authority, environment, or approval date.
- Distinguish decision status from implementation status. Claim implementation only after inspecting relevant changes and observing the required checks. Label unchecked work as planned or unverified, and link to real evidence when available.
- Link the ADR to its source decision, relevant requirements, follow-up records, or implementation evidence when those links exist.

## Maintain the decision history

Respect the repository's amendment policy. When an accepted decision changes, normally create a successor ADR, explain what changed, and link both records. Update an old status only to identify rejection, deprecation, or supersession; retain its original rationale. If the project uses dated amendments instead, follow that policy. Keep any existing decision log until migrated entries and links have been checked; replace only entries that the user asked to migrate.

## Quick reference

| Situation | Action |
|---|---|
| No consequential decision was made | Do not create an ADR. |
| Decision is still being evaluated | Write a proposal or record the open question; do not mark it accepted. |
| User explicitly approves a bounded choice | Mark it accepted for that scope and cite the available approval context. |
| The decision's implementation is unverified | Keep implementation evidence separate and label it unverified. |
| A decision is replaced | Create a linked successor and preserve the earlier rationale. |

## References

This local guide is an original, compact synthesis informed by the [MIT-licensed ADR Authoring skill](https://github.com/magnus919/agent-skills/blob/main/adr-authoring/SKILL.md) and the [Architecture Decision Record community reference](https://github.com/architecture-decision-record/architecture-decision-record).
