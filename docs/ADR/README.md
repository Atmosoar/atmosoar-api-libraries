# Architecture Decision Records

Significant technical decisions are recorded here so the team can understand **why** the system is built the way it is.

## When to Create an ADR

The `/architecture` skill creates an ADR when a decision:
- Affects multiple features or modules (cross-cutting)
- Chooses between viable alternatives (database, auth, framework, deployment strategy)
- Is hard to reverse later
- Would surprise a new team member who doesn't know the history

## Detection triggers

Create or update an ADR when any of these phrases come up in the session:
- "let's record this decision" / "ADR this"
- "we decided to…" / "the reason we're doing X instead of Y is…"
- "why did we choose X?" (read existing ADR before answering)
- A choice is being made between frameworks, libraries, databases, API styles, deployment targets, or auth schemes
- A decision would be non-obvious to a future reader of the code alone

## What NOT to ADR

- Implementation detail within a single module
- Obvious choices with no real alternatives
- Style/naming preferences (those belong in `.claude/rules/`)
- Decisions already covered by an existing ADR (amend or supersede instead)

## ADR Index

| ID | Decision | Status | Date |
|----|----------|--------|------|
| [ADR-001](ADR-001-registry-accessors-with-deprecated-vars.md) | Registry accessors alongside deprecated package-level vars | Accepted | 2026-04-19 |
<!-- New ADRs are added here by /architecture -->

## Next Available ID: ADR-002
