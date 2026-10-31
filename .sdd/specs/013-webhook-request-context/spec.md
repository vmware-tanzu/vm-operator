# Feature Specification: Webhook Request Context Isolation

- **Feature branch**: `bryanv/isolate-webhook-request-context`
- **Fork**: `bryanv/vm-operator`
- **PR target**: `vmware-tanzu/vm-operator`
- **Created**: 2026-10-30
- **Status**: Draft (ticket references pending)
- **Epic**: TBD

## Goals

- Admission requests MUST NOT inherit configuration overrides from previous or concurrent requests, including rejected requests.
- When workload-domain isolation is globally disabled, non-Kubernetes VMs MUST continue using isolation while Kubernetes-node VMs MUST retain the globally disabled behavior.
- Downstream admission operations MUST inherit the HTTP request's cancellation, deadline, and values.
- Requests MUST continue receiving current manager configuration while leaving that configuration unchanged by request-specific overrides.

## Non-goals

No API, schema, feature-gate policy, validation-rule, or caching changes.

## Acceptance criteria

1. Alternating Kubernetes-node and non-Kubernetes admissions produce request-specific isolation settings for create and update, regardless of previous admission failures.
2. Concurrent admissions do not share request-specific overrides or race on the webhook context.
3. Cancelling an HTTP admission request cancels a downstream client operation; the manager remains active.
4. Existing certificate verification and privilege checks retain their behavior.

## Open questions

- [NEEDS CLARIFICATION: maintainer to supply the epic and shipping-code story references before merge.]
