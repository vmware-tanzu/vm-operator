<!--
Thanks for sending a pull request! Authors, human or AI agent, follow .sdd/memory/pull-request-standards.md.

Add one of these icons to the PR title:

    ⚠️ (:warning:, a major or breaking change)
    ✨ (:sparkles:, feature additions)
    🐛 (:bug:, patch and bugfixes)
    📖 (:book:, documentation or proposals)
    🌱 (:seedling:, minor or other)

How to use this template:

    1. The five sections below are required. Never delete their headings. Write "N/A: <reason>" or "NONE" where one truly does not apply.
    2. Add any optional section from the list at the bottom when its trigger applies; some triggers make a section required. You may add other sections when they help a reviewer.
    3. If this is your first PR, read https://vm-operator.readthedocs.io/en/stable/start/contrib/submit-change/. If it is unfinished, prefix the title with "WIP:".
    4. Delete all HTML comments before submitting.
-->

**What does this PR do, and why is it needed?**

<!-- 2-5 sentences, written for a reviewer who has not seen your spec, chat or branch: the problem or goal, the change at a conceptual level, and the effect a user or operator sees. Describe behavior, not files; the diff already lists files. -->

**Which issue(s) is/are addressed by this PR?**

<!-- `Fixes #<issue>` closes a GitHub issue on merge. List every `vmop-NNN` and `.sdd/specs/NNN-slug/` task this PR delivers, and related PRs. Write "N/A" if there are none. -->

Fixes #

**Testing**

<!--
Describe test CASES, not suites or commands: the scenario and inputs, the expected result, and the layer (unit, envtest, vcsim, E2E). Cover the existing behavior this change must not break as deliberately as the new behavior.

New behavior:
- <scenario and inputs> → <expected result> (<layer>)

Existing behavior (regression and compatibility):
- <scenario and inputs> → <expected result> (<layer>)

How verified: only what you actually ran, e.g. `make test`, `make lint-go`, E2E on <environment>.

Not verified: what was not run or only reasoned about (e.g. E2E on a Supervisor, upgrade from a released version). Write "Nothing known" only if you mean it.
-->

**AI usage disclosure**

<!-- "NO", or "YES" and how AI was used (tool, what it wrote) and what a human reviewed or ran personally. An AI agent must not claim a human review that did not happen. -->

**Please add a release note if necessary**:

<!--
    1. Enter your extended release note in the below block. If the PR requires
       additional action from users switching to the new release, please include
       the string "action required".
    2. If a release note is not required, please write "NONE".
-->

```release-note

```

<!--
OPTIONAL SECTIONS: copy any that apply above the release note. "Required when" means you must include it when the trigger applies.

**Behavior: before and after**
When: behavior changes in a way the summary cannot show in a sentence or two, or a design choice is not obvious. Give a before/after table or bullets, and for each decision the reason and the alternative you rejected.

**Scope**
When: the PR is one of several for a story, or work was deferred. In this PR / not in this PR, with where each deferred item is tracked (task ID or vmop-NNN).

**Compatibility and upgrade**
Required when: the PR changes anything under api/, CRDs, conversion, webhooks, defaults, or how existing VMs are reconciled. Answer each line, or write "N/A: <reason>":
- Older API versions and older clients (round trips, older writers):
- Existing objects after upgrade:
- Version skew (operator, vCenter, other Supervisor components on different versions) and rollback:
- Feature or capability gate:

**Reviewer guide**
When: the change is large or mixes hand-written and generated code. Where to start, which files are generated (and the command that regenerates them), and the risky or subtle parts.

**Special notes for your reviewer**
Anything else a reviewer should know.
-->
