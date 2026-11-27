---
name: spec-exec-summary
description: Generate a short executive summary (% complete, done, remaining, bottom line) for an SDD spec under .sdd/specs/NNN-slug/. Use when the user asks for an "exec summary", "status", "progress", or "where are we" on a spec, feature, or plan that lives in this repo's .sdd tree.
---

# Spec executive summary

Produce a tight, leadership-readable status update for one feature under
`.sdd/specs/NNN-slug/`, in the voice of the example below — plain language
about *capabilities delivered* and *what's left*, not a restatement of task
IDs.

## 1. Resolve the target spec

Args (if given) may be a slug, a zero-padded number, a partial name, or
"latest". Resolve against `.sdd/specs/*`:

- Exact or unique substring match on the directory name → use it.
- No arg / "latest" → pick the spec whose `tasks.md` has the most recent
  mtime.
- Ambiguous or no match → list the candidate directories (name + title line
  from their `spec.md`) and ask the user which one, via AskUserQuestion.

Do not guess silently when more than one spec plausibly matches.

## 2. Read the source files

For the resolved `.sdd/specs/NNN-slug/`, read:

- `tasks.md` — the source of truth for completion state.
- `spec.md` — title, Goals, Non-Goals (for scope callouts like "X excluded
  from scope"), acceptance criteria (for judging what's user-facing/high
  value).
- `plan.md` — story/phase rationale, so remaining-work bullets can describe
  *what a story delivers and why it matters*, not just its task text.

Skip `research.md` unless a specific open question needs it.

## 3. Parse `tasks.md`

- Track `## Phase N — Title` and `### Story SN — Title (ticket)` headers as
  the grouping structure.
- Every `- [ ]` / `- [x]` line is one task. Ignore prose/notes lines (e.g.
  blockquote implementation notes, "Gate:" lines) — they are not tasks.
- Tally checked vs. total tasks overall, and per story/phase.
- Classify each story: **Done** (all tasks checked), **Not started** (none
  checked), **In progress** (partial — note the fraction).
- Note tickets/PRs referenced on *done* tasks (e.g. `[PR #1695 / vmop-3740]`)
  and collect the distinct PR numbers per completed capability — each
  **Done** bullet should end with the PR link(s) that shipped it, so the
  summary stays concrete about what shipped. Dedupe PRs that recur across
  several tasks in the same bullet (list once, not once per task).
- Note any story marked retired/"Won't Implement" in prose (e.g. "Story S4
  was retired") — exclude it from both the denominator and the remaining
  list, but keep the rationale in mind for scope callouts.

Compute overall completion as `checked / total`, rounded to the nearest 5%.
If a phase is explicitly gated (a "Gate: ... must be complete before Phase N
begins" line) and unmet, say so — it blocks everything after it regardless
of how much of the later phase's tasks look plannable.

## 4. Write the summary

Follow this structure and tone exactly (see the worked example below):

```
Exec Summary — <Feature title> (<scope callout if any, e.g. "X excluded from scope">)

Where we are: ~<N>% complete.

Done:
<3-6 bullets, one capability per bullet, in plain language — what now
works, not which tasks are checked. Merge multiple completed tasks into one
statement of delivered capability. Mention a controller/webhook/CRD by name
once, not by file path. End each bullet with the PR link(s) that delivered
it, e.g. "(PR #1695, PR #1711)" — omit only if tasks.md has no PR reference
for that capability.>

Remaining for next release:
<numbered list, dependency order, one item per not-started/in-progress
story or phase. Each item: what it does and why it's needed, in a sentence
or two. Call out any unmet Gate that blocks the rest.>

Bottom line: <1-3 sentences: what to finish first, and — if the spec's
acceptance criteria describe user/customer-observable behavior that hasn't
started — say explicitly that the customer-facing value is still ahead.>
```

Rules for good output:

- **Done** and **Remaining** describe capabilities, not task checkboxes.
  Never just paste task text or `[x]`/`[ ]` markers into the summary.
- Order **Remaining** by actual dependency order from `plan.md`/`tasks.md`
  phase structure, not alphabetically or by story number if the plan
  reorders them.
- If one remaining story is clearly the "customer actually experiences
  this" layer (enforcement, an admission path, a user-visible API) — call
  that out explicitly in the bottom line, the way the example does.
- Keep it short: this is a status update someone reads in under a minute,
  not a re-derivation of the plan.
- Do not write the summary to a file unless the user asks — the answer is
  the chat response.

## Worked example

Input: `.sdd/specs/001-class-policy-resize/` (Epic vmop-3331), 1 of 3 phases
in flight, Story S4 retired, Story S10 (SR-IOV) deferred out of this
release's scope per `research.md` Finding 7.

```
Exec Summary — VM Service Class Policy and Resize (SR-IOV excluded from scope)

Where we are: ~35% complete.

Done:
Feature gate and CRDs installed and wired into the platform. (PR #1649, PR #1667)
Zone controller automatically provisions the cluster capability record (ConfigTarget) and a default policy object when a Zone is created. (PR #1695)
ConfigTarget webhook (validation) and the core controller that reads basic cluster capabilities (CPU/memory limits, security features) from vCenter — done and merged. (PR #1711)

Remaining for next release:
1. Max hardware version detection: Still needs to determine whether we need to traverse through the hosts for this or getting the value from the cluster resource is sufficient.
2. VM Config Options & Guest Options controllers: These translate vSphere capabilities into per-hardware-version, per-guest-OS options that policies reference.
3. VM Config Policy controller: This is the object that syncs cluster capability data into a usable, tenant-facing policy.
4. VM admission enforcement: This is the actual policy enforcement on VM create/update (mode restrictions, config restrictions, hardware-version limit) — the core value of this feature.

Bottom line: finish max-hardware-version detection, then build the three downstream controllers, then wire up enforcement. None of the enforcement layer — the part customers actually experience — has started yet.
```

## Multiple specs / portfolio view

If the user asks for a summary across *all* in-flight specs, run steps 2-4
independently per spec directory and present each as its own block under a
`## <spec title>` heading — do not average completion percentages across
specs into one number.
