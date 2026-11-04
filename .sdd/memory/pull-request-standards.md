# Pull requests

<!--
Keep instructions for the model in plain text: Claude Code strips HTML comments before loading this file. The canonical template is .github/PULL_REQUEST_TEMPLATE.md.
-->

## Creating

- **Title**: add one of these icons from [`.github/PULL_REQUEST_TEMPLATE.md`](../../.github/PULL_REQUEST_TEMPLATE.md): ⚠️ (`:warning:`, a major or breaking change), ✨ (`:sparkles:`, feature additions), 🐛 (`:bug:`, patch and bugfixes), 📖 (`:book:`, documentation or proposals), 🌱 (`:seedling:`, minor or other). If the PR is unfinished, prefix the subject with `WIP:`.
- **Description**: use the template below, and delete all HTML comments from `.github/PULL_REQUEST_TEMPLATE.md` if you start from that file. In the release-note block, include the string `action required` if users switching to the new release must take additional action; if no release note is required, write `NONE`.

````
**What does this PR do, and why is it needed?**

**Which issue(s) is/are addressed by this PR?** *(optional, in `fixes #<issue number>(, fixes #<issue_number>, ...)` format, will close the issue(s) when PR gets merged)*:

Fixes #


**Are there any special notes for your reviewer**:

**Please add a release note if necessary**:

```release-note

```
````

## Reviewing

- Submitting a pull request using AI tooling does not obviate a reviewer of the responsibility to validate the generated comments. It is **not** the responsibility of the change's author to spend time determining which feedback is actually valid.
