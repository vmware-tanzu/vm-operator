# Plan: E2E Support for Isolated Testbeds

## Approach

- `hack/e2e/isolated.sh` (sourced by `setup-testbed-env.sh`): detect isolation (a jumphost `network[]` entry, or a declared isolated network prefix that contains the vCenter address setup already parsed), discover the jumphost from the testbed JSON, open an SSH ControlMaster with a SOCKS5 listener on a free port inside a per-run `/tmp/e2e-iso.*` directory, write an ssh_config whose `ProxyCommand` multiplexes over the master, and stop the tunnel.
- `setup-testbed-env.sh`: bring the tunnel up before the first vCenter operation, install `sshpass` first, derive the management CIDR/prefix from the isolated network prefix (octet or CIDR form) rather than hardcoding `10.0.0.0/8`, and set `HTTP(S)_PROXY` to the SOCKS URL until a local forward to the gateway's Squid is verified (T008), never to the gateway address itself. Only stop a tunnel this invocation opened on failure; a repeat setup in the same shell stops the earlier tunnel and unsets everything it exported. `E2E_ISOLATED`/`E2E_JUMPHOST_*` are inputs only.
- `proxy.sh`, `kms.sh`: honor `E2E_SSH_CONFIG` and `E2E_MGMT_IP_PREFIX`.
- `run-e2e.sh`: own teardown (`EXIT`) after support-bundle collection and the result callback. `INT`/`TERM` traps only record the signal, so an interrupted run still collects the bundle and reports before exiting 130/143.
- Go: `ssh` dials through `proxy.Dial` (`ALL_PROXY`) with a context deadline, so the TCP connect, the SOCKS5 negotiation, and the SSH handshake are all bounded; the gateway identity comes from `GATEWAY_IP` and is never parsed out of a scheme-bearing `HTTP_PROXY`; `kubectl-vsphere` receives `ALL_PROXY`.

## Constitution check

No API, controller, or webhook changes. E2E-harness only; the only `go.mod` change promotes `golang.org/x/net` to a direct use.

## Test strategy

- Ginkgo unit coverage for gateway resolution and for a dial against a stalled SOCKS5 proxy returning within the timeout (`infrastructure/vsphere/ssh`).
- Real-testbed run of `make e2e-smoke` on an isolated VDS testbed.
