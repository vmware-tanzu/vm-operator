---
name: triage-prod-vmsvc-reco
description: >-
  Triage VM Service E2E pipeline failures end-to-end: collect gce2e logs,
  parse JUnit timings against wcp.yaml timeouts, categorize failures, decide
  whether to dig into operator logs / support bundle / ESX logs, and escalate
  when out of scope. Use when the user asks to investigate, triage, or root-cause
  any prod-vmsvc-reco-* pipeline build failure.
args: |
  Jenkins pipeline URL (e.g., https://jenkins-vcfwcp.devops.broadcom.net/job/prod-vmsvc-reco-main/2207/)
disable-model-invocation: true
---

# VM Service E2E — Failure Triage

## Overview

Always work **data-first**. Present a high-level summary first, then wait for the user to pick which failure(s) to investigate. Never auto-advance to deep log analysis without user direction.

```
Pipeline build URL
    │
    ▼
Step 1 — Classify the build (INFRA vs PRODUCT)
    │
    ├── INFRA failure → Step 7 (Escalation)
    │
    └── PRODUCT: gce2e failure
            │
            ▼
        Step 2 — Run fetch-gce2e-logs.py
            │   Downloads: JUnit XML + per-suite vm-operator logs
            │
            ▼
        Step 3 — Present HIGH-LEVEL SUMMARY  ◄── STOP HERE
            │   • Compact table: #, test, suite, category, time vs timeout
            │   • 2-3 line error snippet per failure (from JUnit message)
            │   • Group INFRA flakes separately from PRODUCT failures
            │   • Ask the user: "Which failure(s) should I investigate?"
            │
            │   ── WAIT FOR USER DIRECTION ──
            │
            ▼ (only after user picks)
        Step 4 — Deep dive: timeline analysis on operator log
            │   Targeted to the specific failure(s) the user asked about
            │
            ▼
        Step 5 — Decision: support bundle needed?
            │   See matrix below
            │
            ├── Not needed → write RCA
            │
            └── Needed → Step 6 (fetch-bundle.py)
                            │
                            └── ESX logs needed? → Step 7 (Escalation)
```

---

## Step 1 — Classify the build

```bash
# Get the pipeline category from triage report or Jenkins console
curl -sf "https://jenkins-vcfwcp.devops.broadcom.net/job/<JOB>/<BUILD>/consoleText" \
  --globoff | grep -E "FAILURE|gce2e|OVA|Import|checkout|testbed" | head -20
```

| Signal in console | Category | Next step |
|-------------------|----------|-----------|
| `run-guest-clusters-end-to-end-tests #NNN` found | PRODUCT: gce2e | → Step 2 |
| `OVA\|transferring file\|InternalServerError` | INFRA: OVA Import | → Step 7 |
| `Maximum checkout retry\|checkout` | INFRA: Git Checkout | → Step 7 |
| `Deploy Testbed stage failed` | INFRA: Testbed Creation | → Step 7 |
| `WCP Sanity Tests failed` | INFRA: WCP Sanity | → Step 7 |

---

## Step 2 — Collect gce2e artifacts

The skill ships `scripts/fetch-gce2e-logs.py`. It:
1. Finds the gce2e build number from the pipeline console.
2. Downloads the JUnit XML and parses every failing test with its **actual elapsed time**.
3. Looks up the configured timeout for each test from `wcp.yaml`.
4. Downloads the **targeted vm-operator log per test suite** (already archived by gce2e — no support bundle needed at this stage).
5. Prints a structured triage summary.

```bash
python3 .cursor/skills/triage-vm-service-e2e/scripts/fetch-gce2e-logs.py \
    https://jenkins-vcfwcp.devops.broadcom.net/job/<JOB>/<BUILD#> \
    --out /tmp/gce2e-debug
```

Output directory structure:
```
/tmp/gce2e-debug/
  junit_vmservice.xml
  v1alpha5-vmsnapshot/logs-vmware-system-vmop-*.log
  vm-encryption/logs-vmware-system-vmop-*.log
  vm-lcm/logs-vmware-system-vmop-*.log
  vm-hardware/logs-vmware-system-vmop-*.log
  vm-guest-customization/logs-vmware-system-vmop-*.log
  ...
```

---

## Step 3 — Present high-level summary  ⚑ STOP HERE

After the script completes, emit a summary in this exact format, then **stop and ask the user which failure(s) to investigate**. Do not proceed to Step 4 until the user responds.

### 3a — Output format

**Header line** (one line):
```
Pipeline: <job-name> #<build>   Topologies: VDS · NSX · NSX-VPC · ...   Total failures: N
```

**Failure table** (one row per failing test):

| # | Test (shortened) | Suite | Cat | Actual / Timeout | Error snippet |
|---|-----------------|-------|-----|-----------------|---------------|
| 1 | `successfully create…snapshots` | `v1alpha5-vmsnapshot` | ⏱ TIMEOUT | 10m02s / 10m | `Timed out waiting for VirtualMachineSnapshot…conditions: []` |
| 2 | … | | | | |

Rules for the table:
- Shorten test names to the last 60 characters if needed (they're long Ginkgo paths).
- **Error snippet**: first 120 chars of the JUnit `<failure>` message, newlines replaced by `↵`.
- Group rows: PRODUCT failures first, then INFRA flakes (mark INFRA rows with 🔧).
- If multiple topologies failed on the same test, collapse to one row and note topologies: `VDS · NSX-VPC`.

**Category key** (print inline under the table):
- ⏱ TIMEOUT — elapsed ≈ configured timeout; condition never became True
- ❌ ASSERTION — wrong value returned; check test logic / operator state
- 🔧 INFRA — SSH / OVA / testbed error; likely infra flake, escalate
- 🔴 HANG — elapsed >> timeout; missing hard timeout guard

### 3b — Cross-topology pattern check

After the table, print one sentence stating whether the same test failed across ≥2 topologies (strong signal of a product bug, not a testbed flake).

### 3c — Stop and ask

End with exactly:
```
Which failure(s) should I investigate? (e.g. "all", "#1", "#1 and #3", or "skip INFRA")
```

**Do not read any operator logs, do not grep any files, do not open any source files until the user replies to this prompt.**

---

## Step 4 — Deep dive: timeline analysis on operator log  (on user request)

Run this step **only** for the specific failure number(s) the user named. Use the operator log downloaded in Step 2 — no support bundle required yet.

**Timeout config reference** (from `test/e2e/vmservice/config/wcp.yaml`):

| Operation | Interval key | Timeout | Poll |
|-----------|-------------|---------|------|
| VM snapshot ready | `wait-virtual-machine-snapshot-condition` | **10m** | 5s |
| VM created | `wait-virtual-machine-creation` | **5m** | 10s |
| VM power state | `wait-virtual-machine-powerstate` | **5m** | 10s |
| VM condition update | `wait-virtual-machine-condition-update` | **5m** | 10s |
| VM IP address | `wait-virtual-machine-vmip` | **5m** | 10s |
| VM image creation | `wait-virtual-machine-image-creation` | **10m** | 5s |
| SSH / login retry | `login-retry-timeout` | **5m** | 10s |
| PVC attachment | `wait-pvc-attachment` | **5m** | 10s |
| VM group condition | `wait-virtual-machine-group-condition-update` | **6m** | 5s |
| CNS batch attach | `wait-virtual-machine-cns-node-batch-attachment-creation` | **10m** | 5s |
| Backup complete | `wait-backup-to-complete` | **10m** | 5s |
| Jumpbox SSH ready | `wait-jumpbox-sshpass-ready` | **30m** | 10s |

```bash
VMOP_LOG="/tmp/gce2e-debug/<suite>/logs-vmware-system-vmop-*.log"

# 1. Find all errors for the failing VM (name comes from test / JUnit message)
rg "terminal error|Reconciler error" "$VMOP_LOG" | head -40

# 2. Build a timeline for a specific VM
VM="vm-snapshot-abc1"
rg "\"$VM\"" "$VMOP_LOG" | head -40

# 3. Find the first reconcile timestamp vs. the error timestamp
# → calculate: create → first reconcile → condition stuck → timeout
rg "\"$VM\"" "$VMOP_LOG" | awk '{print $1, $0}' | sort | head -20

# 4. Check for the specific stuck condition
rg "conditions.*\[\]|hasPendingCRVs|ErrPendingBackfill|SubnetPort|OpaqueNetwork" "$VMOP_LOG" | head -20
```

**Pinpointing which phase caused the timeout:**

| Elapsed before error | Likely bottleneck phase |
|----------------------|------------------------|
| < 30s | Immediate rejection (bad spec, missing resource, API error) |
| 30s – 2m | Early reconcile block (storage policy, NSX SubnetPort not ready) |
| 2m – (timeout − 10s) | Condition set but then reverted (race), or VM stuck in vSphere task |
| ≈ configured timeout | Condition never set at all; operator loop not triggering |

---

## Step 5 — Support bundle decision matrix

| Operator log shows | Support bundle needed? | ESX logs? |
|--------------------|------------------------|-----------|
| `terminal error: network interface is not ready` | ✅ YES — need NSX pod logs | ❌ No |
| `Timed out … conditions: []` + no operator errors | ✅ YES — condition never set | ❌ No |
| `ErrPendingBackfill` / `missing controller bus number` | ✅ YES — CSI / backfill state | ❌ No |
| `OpaqueNetworkBackingInfoImpl` / `InvalidArgument` | ❌ No — test/product bug | ❌ No |
| `409 conflict` / `resourceVersion` | ❌ No — test uses Update not Patch | ❌ No |
| `ManagedEntityStatus: red != green` | ❌ No — check KMS / encryption state | ❌ No |
| `vSphere MethodFault` / `InvalidArgument(path)` | ✅ YES | ⚠️ Maybe |
| `disk … is not restored` / VSLM error | ✅ YES — snapshot delta race | ❌ No |
| OVA import / `transferring file` error | 🚫 ESCALATE — infra issue | 🚫 ESCALATE |
| `esx\|kernel\|storage IO` error in VC logs | 🚫 ESCALATE — needs ESX team | ✅ If escalating |

**When ESX logs ARE needed** (rare):
- vSAN I/O errors during OVA transfer
- ESX kernel panic affecting VM power-on
- Storage controller bus-number issues not explained by operator log

ESX logs are in `esx-host-*.tar.gz` inside the support bundle (large — extract only if justified).

---

## Step 6 — Support bundle download (only when Step 5 says YES)

```bash
python3 .cursor/skills/triage-vm-service-e2e/scripts/fetch-bundle.py \
    https://jenkins-vcfwcp.devops.broadcom.net/job/<JOB>/<BUILD#> \
    --out /tmp/wcp-sb
```

The script outputs ready-to-paste shell variables:
```
SB="/tmp/wcp-sb/wcp-agent-<hash>-<ts>/"
VMOP_LOG="$SB/var/log/pods/vmware-system-vmop-.../manager/0.log"
NSX_POD="$SB/var/log/pods/vmware-system-nsx-operator-.../"
```

**Search patterns after pasting the vars:**

```bash
# Decompress rotated logs
gunzip "$SB/var/log/pods/vmware-system-vmop-"*/manager/*.log.*.gz 2>/dev/null || true

# SNAP / EXACT_TIMEOUT — snapshot condition never set
rg "VirtualMachineSnapshot|hasPendingCRVs|snapshot" "$VMOP_LOG" | head -40

# HW — backfill / SubnetPort
rg "terminal error|ErrPendingBackfill|SubnetPort|bus number" "$VMOP_LOG" | head -40

# LCM — NIC backing
rg "OpaqueNetwork|InvalidArgument|deployOVF" "$VMOP_LOG" | head -30

# NSX SubnetPort
rg "SubnetPortNotReady|empty NSX resource" "$NSX_POD"/*/0.log | head -20
```

**Key log signals:**

| Signal | Meaning |
|--------|---------|
| `terminal error: network interface is not ready` | NSX SubnetPort not realized; reconcile exits, no requeue |
| `ErrPendingBackfill` / `Backfilled unmanaged volume` | Backfill ran once; condition not yet True |
| `PolicyEvaluation … not ready` | Storage policy gate blocking reconcile |
| `VM has task` + taskID | vSphere task in flight, reconcile skipped |
| `noRequeueReason="backed up vm"` | Backup reconcile path — main reconcile not running |
| `hasPendingCRVs=true` | CnsRegisterVolume still in flight; snapshot must wait |

---

## Step 7 — Escalation / Human intervention required

Call out the following explicitly. **Do not dig deeper in support bundle or code for these.**

| Signal | Category | Action |
|--------|----------|--------|
| `Error transferring file *.ova` | INFRA: OVA Import | File Nimbus/testbed bug; check if content library vSAN is healthy |
| `Maximum checkout retry` | INFRA: Git | Check GitHub Enterprise / network; pipeline team |
| `Deploy Testbed stage failed` | INFRA: Testbed | Nimbus scheduling/capacity issue; pipeline team |
| `test ran >> configured timeout` (HANG) | Test bug | Add hard timeout + `Skip` guard in `BeforeAll`; missing timeout is a P0 pipeline blocker |
| NSX `SubnetPort` never realized after support bundle analysis | Product / NSX team | File bug with NSX operator team; include SubnetPort YAML + NSX operator logs |
| KMS / encryption status `red` | Product / KMS config | Verify KMS cluster is reachable from testbed; check `wait-kms-cluster-ready` |
| ESX kernel panic / storage I/O | Infra / ESX team | Include esx-host logs; escalate to storage/ESX team |

---

## Step 8 — Topology-specific notes

| Check | VDS | NSX | NSX-VPC |
|-------|-----|-----|---------|
| `VirtualMachineUnmanagedVolumesBackfilled` fails | rare | 100% | 100% |
| `OpaqueNetworkBackingInfoImpl` rejection | no | yes | yes |
| `terminal error: network interface not ready` | no | possible | yes |
| VM-VPC webhook errors | no | no | yes |
| OvfEnv cloud-init SSH race | yes (no retry) | no | no |

If a failure is topology-specific:
1. Confirm topology from the job name (`prod-vmsvc-reco-vds`, `prod-vmsvc-reco-nsx`, `prod-vmsvc-reco-nsx-vpc`).
2. Check if a `skipper.SkipUnless…` / `skipper.SkipIf…` guard is needed in the test.
3. Apply fixes to both `vm-operator/test/e2e/vmservice/` and `vks-gce2e/test/vmservicee2e/` if the file exists in both.

---

## Step 9 — Write RCA and action items

After identifying root cause:

1. **Code fix**: patch test or operator as needed; see type-specific notes below.
2. **Test guard**: add `skipper` call if feature/topology is unsupported.
3. **RCA write-up**: update `triage-report-<date>.md` — add error evidence, log excerpts, affected builds, fix.
4. **Action item**: add to Action Items table with priority (🔴/🟡/🟢), owner, and due date.

---

## Appendix — Type-specific fix pointers

### SNAP (`VirtualMachineSnapshot` conditions=[] timeout)
- Source: `vm_snapshot.go`
- Check `BeforeAll` for indefinitely blocking `WaitOnVirtualMachineCondition` → add ≤ 10-min hard timeout + `Skip`.
- Check `WaitForVMCnsRegisterVolumesRegistered` is called before any snapshot (VSLM `RegisterDisk` fails if CRV in flight).
- Snapshot delta leaves a child disk that makes `RegisterDisk` reject with `InvalidArgument("path")`.

### HW (`VirtualMachineUnmanagedVolumesBackfilled` timeout)
- Source: `vm_hardware.go` — `Multiple VMs sharing MultiWriter PVCs`
- `ErrPendingBackfill` exits reconcile **without requeue** — condition stuck until next spec change.
- NSX: `terminal error: network interface not ready` → NSX SubnetPort never realized.
- Code: `pkg/*/unmanagedvolumes_backfill.go` — condition set True only when `updatedSpec=false`.

### LCM (`OpaqueNetworkBackingInfoImpl` / VM creation timeout)
- Source: `virtualmachinelcm.go`
- `fast-deploy: "false"` annotation forces OVF/VDCS path → VDCS rejects NSX NIC backing.
- Add `skipper.SkipIfNetworkingIsVPC(...)` or supply VPC-compatible NIC spec.

### GCUST (`VM-GUEST-CUSTOMIZATION` SSH failure)
- VDS: `VerifyLoginAndRunCmdsInVDSSetup` has no retry — wrap in `Eventually(..., login-retry-timeout...)`.
- `disk is not restored`: call `WaitForVMCnsRegisterVolumesRegistered` before `DeleteVMResource`.

### ENCRYPT (`VM-ENCRYPTION` assertion fail — `red != green`)
- KMS status check fails; verify KMS cluster is reachable from testbed.
- Re-encryption tests: use `wait-encrypted-virtual-machine-powerstate` (`["10m","10s"]`), not the default 5m.

### VIADM (`VI-ADMIN-RegisterVM` 409 conflict)
- Use `Patch` with `MergeFrom(base)` instead of `Update`.
- Remove `Eventually` wrapper hiding the 409 — `Patch` is idempotent.

