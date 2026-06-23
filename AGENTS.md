# AI Code Review Instructions — vm-operator

## Authoritative standards

All coding standards, architectural rules, process requirements, and feature specs live under `.sdd/`. Start here:

**[`.sdd/INDEX.md`](.sdd/INDEX.md)** — directory index of all memory rules and per-feature specs, with one-line descriptions and direct links.

Before reviewing a PR:
1. Read the memory rules listed in the index (they apply to every change).
2. Check whether the changed files correspond to an in-progress spec. If so, read that spec's `spec.md` and `plan.md` — the spec is the acceptance criteria for the review.

---

## Review priorities

The `.sdd/memory/` files define what is correct. Use this section only to calibrate severity and avoid false positives — do not re-derive rules from it.

### Flag as high priority

- Architecture boundary violations: business logic in `controllers/`, vSphere API calls outside `pkg/providers/vsphere/`
- API breaking changes without a version bump or conversion webhook
- CRD manifests not regenerated after API type changes
- VM update reconcile order changed — see `operator-best-practices.md`; the sequence has hidden inter-step dependencies
- Conditions used to gate reconciliation flow rather than communicate state
- Missing or incorrect copyright header on new Go files
- Ticket or wiki references that do not follow the format in `constitution.md`
- New cluster-observable behavior without E2E coverage in the same PR
- Non-trivial change (new CRD field, controller, webhook, cross-package refactor, feature flag) without a corresponding `.sdd/specs/` entry
- Code implementing an in-progress spec that is not guarded by a feature flag — check `pkgcfg.FromContext(ctx)` is used to gate the new behavior

### Flag as medium priority

- Public functions/methods missing godoc
- Errors not wrapped with context
- Missing `status.observedGeneration` update or `Ready` condition in a new controller
- Patch helper not deferred, or `reterr` named return missing from `Reconcile`
- `pkgcfg.JoinContext` missing at the top of `Reconcile`

### Do not flag

- `.cursor/rules/` contents — intentional thin proxies to `.sdd/memory/`, not meant to be expanded
- Markdown files with long lines — no hard wrapping is intentional per the constitution (code fences excepted)
- The 11-step VM reconcile order itself — do not suggest reordering
- Single-letter receiver names (`r`, `v`, `w`) — intentional per architectural standards
- `external/` module contents — vendored external API types, not modified directly
