# VM Operator Governance

This document defines the project governance for VM Operator.

## Overview

VM Operator is committed to building an open, inclusive, productive, and self-governing open source community focused on enabling the management of virtual machines on Kubernetes. The community is governed by this document, with the goal of defining how community members can contribute and how decisions are made.

## Code of Conduct

VM Operator follows the [Code of Conduct](CODE_OF_CONDUCT.md), which all community members, maintainers, and contributors must adhere to.

## Community Roles

* **Users**: everyone who interacts with the project, e.g. via the mailing list, community meetings, issues, or discussions.
* **Contributors**: anyone who contributes to the project, including code, documentation, tests, issues, or discussions.
* **Maintainers**: contributors who have write access to the repository and are listed in [MAINTAINERS.md](MAINTAINERS.md). Maintainers are responsible for the overall health and direction of the project.

## Decision Making

VM Operator uses a **lazy consensus** model for day-to-day decisions:

* Anyone can propose a change by opening a pull request or issue.
* A pull request may be merged once it has approval from at least one maintainer and no maintainer has raised an unresolved objection.
* Substantial changes (new APIs, breaking changes, or changes to project governance) should be raised as a proposal (issue or design doc linked from an issue) and discussed in a community meeting or the mailing list before implementation, per the Spec-Driven Development workflow described in `.sdd/`.
* If consensus cannot be reached through discussion, a supermajority vote of maintainers decides the matter.

## Becoming a Maintainer

To become a maintainer, a contributor should:

1. Have a sustained history of quality contributions (code, reviews, documentation, or community support) over a meaningful period of time.
2. Demonstrate good judgment in code review and design discussion.
3. Be nominated by an existing maintainer, with the nomination sent to the [mailing list](https://groups.google.com/g/vm-operator-dev) or raised in a community meeting.
4. Receive no objections from existing maintainers within one week, or be approved by a supermajority vote of existing maintainers if an objection is raised.

New maintainers are added to [MAINTAINERS.md](MAINTAINERS.md) and [CODEOWNERS](CODEOWNERS).

## Removing a Maintainer

Maintainers may step down at any time by opening a pull request removing themselves from [MAINTAINERS.md](MAINTAINERS.md) and [CODEOWNERS](CODEOWNERS).

Maintainers who are inactive for an extended period (e.g. no reviews, commits, or community participation for six months) or who violate the [Code of Conduct](CODE_OF_CONDUCT.md) may be removed by a supermajority vote of the remaining maintainers.

## Community Meetings

The project holds regular community meetings, open to anyone. See the [public calendar](https://calendar.google.com/calendar/embed?src=a71d7f1058d04da71b6209546308c54d9988ae4357812fa91188eed393d57618%40group.calendar.google.com&ctz=America%2FLos_Angeles) for the schedule and join via [Google Meet](http://meet.google.com/eoa-zodx-stt). Agendas and notes are kept in the [community doc](https://docs.google.com/document/d/1MfTX2_F7rgtr_e55S5emv-OSmhenJhFMpdYqSIyED2w/edit?tab=t.0).

## Communication Channels

* Mailing list: [vm-operator-dev](https://groups.google.com/g/vm-operator-dev)
* Slack: [#ug-vmware](https://kubernetes.slack.com/messages/ug-vmware)
* Issues and pull requests: [GitHub](https://github.com/vmware-tanzu/vm-operator)

## Amendments

Changes to this governance document require approval from a supermajority of maintainers and should be discussed in a community meeting or on the mailing list before merging.
