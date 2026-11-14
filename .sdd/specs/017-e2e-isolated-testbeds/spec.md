# Feature Specification: E2E Support for Isolated Testbeds

- **Created**: 2026-11-14
- **Status**: In Progress
- **Epic**: vmop-4430

---

## Summary

VM Operator E2E setup and several framework helpers assume direct access to the vCenter and Supervisor management endpoints. On a Nimbus isolated testbed those endpoints sit behind a single perimeter jumphost, and restricted UTS containers cannot rely on privileged routing or `sshuttle`. This feature lets the existing E2E suites bootstrap and run against isolated testbeds using application-layer connectivity, independent of `NETWORK=vds|nsx`, while preserving ordinary testbeds and local development.

## Goals

- Bootstrap from a direct `testbedInfo.json` or a UTS `deliverable_blob` without direct runner access to private endpoints.
- Route shell SSH/SCP, Go SSH, and HTTP clients through the jumphost.
- Need no `CAP_NET_ADMIN` or other privileged networking (the tunnel is plain unprivileged `ssh`).
- Fail setup with useful diagnostics when required connectivity is missing; keep the tunnel through support-bundle collection; clean up on completion or interruption.
- Leave ordinary testbeds and concurrent runs unaffected.

## Non-goals

- Provisioning the testbed or the jumphost.
- Veeam appliance-to-vCenter/ESXi reachability.
- Running the E2E image as a UID with no `/etc/passwd` entry. All shell `ssh`/`scp` in the harness, isolated or not, already assumes the UID has an entry (OpenSSH exits with `No user exists for uid`); this is a pre-existing property of the image and out of scope here.

## Acceptance criteria

- [x] Isolated testbeds bootstrap from a direct `testbedInfo.json`.
- [ ] Bootstrap from a UTS `deliverable_blob` (detection looks at both the unwrapped and raw JSON, but a real blob has not been exercised).
- [x] Shell SSH/SCP (ssh_config `ProxyCommand` over a ControlMaster) and Go SSH (`ALL_PROXY` dialer) use the jumphost; HTTP clients use the SOCKS tunnel.
- [x] Isolated VDS: `make e2e-smoke` passes VM lifecycle and guest access (16 of 17 specs; the one failure is a deterministic VM-group power-ordering assertion that involves no connectivity).
- [x] KMIP provider validated on isolated VDS with `PIP_INDEX_URL` set to the corporate PyPI mirror (`gce2e-standard` and `gce2e-native` green).
- [ ] Isolated NSX/VPC and Packer are validated.
- [x] Kubernetes exec/logs/port-forward validated. SOCKS cannot carry them (kubectl rejects `socks5h://` for both WebSocket and SPDY), so HTTP clients use an HTTP CONNECT proxy reached through an SSH local forward to the gateway's Squid.
- [x] The jumphost tunnel is supervised and recovers from a dropped connection (validated with `kill -9` of the master).
- [x] No files outside a per-run directory, no fixed ports, no `CAP_NET_ADMIN`.
- [x] Tunnel is stopped by `run-e2e.sh` on exit/interrupt and by a failed setup.
