# Storage quota webhook watch consolidation

- **Feature branch**: `bryanv/spq-webhooh-watch`
- **Created**: 2026-10-16
- **Status**: Draft
- **Epic**: vmop-4310

## Goals

Consolidate webhook CA propagation into normal storage quota reconciliation. A relevant webhook event fans out to every watched quota; the existing quota reconciler remains responsible for creating and updating managed usages.

## Acceptance criteria

- Creating or updating the VM Operator validating webhook configuration enqueues quotas in every watched namespace, including quotas with no existing usage.
- Quota reconciliation updates VM usage CA bundles and creates/updates snapshot usages only when VMSnapshots is enabled. Other extensions remain outside quota management.
- Matching desired state causes no usage writes. Usage status and unrelated metadata remain intact; controller-owned spec fields and ownership converge through normal quota reconciliation.
- The first nonempty webhook CA bundle takes precedence; the existing annotated Secret fallback remains supported.
- CA lookup, storage class lookup, and usage write failures remain retryable for each enqueued quota without another webhook event.
- The mapper reads only quotas. It does not fetch the CA, list usages, or inspect usage owners. An unexpected quota-list failure is logged and returns no requests, following the existing mapper contract.
- Unrelated webhook configurations, deletion events, generic events, and unchanged resource versions do not trigger fan-out.
- The webhook watch and normal quota reads share the manager's full-object cache.

## Supported scope and behavior differences

CA propagation applies to usages reached by a live quota and a matching storage class. Existing usages outside this scope no longer receive a separate CA-only repair. Webhook events now run normal quota reconciliation, including controller-owned spec correction and the existing snapshot extension migration. These differences are accepted as part of the requested mapper/fan-out design.

## Non-goals

No API, CRD, admission, quota accounting, Secret watch, namespace scope, feature flag, API-reader, or metadata-watch changes. No independent orphan-usage repair path.

## Open questions

- [NEEDS CLARIFICATION: Assign the shipping task ticket under vmop-4310 before merge.]
