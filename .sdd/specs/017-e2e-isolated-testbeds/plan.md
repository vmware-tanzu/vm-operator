# Plan: E2E Support for Isolated Testbeds

## Approach

- `hack/e2e/isolated.sh` (sourced by `setup-testbed-env.sh`): detect isolation, discover the jumphost from the testbed JSON, open an SSH ControlMaster with a SOCKS5 listener on a free port inside a per-run `/tmp/e2e-iso.*` directory, write an ssh_config whose `ProxyCommand` multiplexes over the master, and stop the tunnel.
- `setup-testbed-env.sh`: bring the tunnel up before the first vCenter operation, install `sshpass` first, derive the management CIDR/prefix from `isolatedNetworkOptions.networkPrefix` rather than hardcoding `10.0.0.0/8`, and keep `HTTP(S)_PROXY` as the SOCKS URL instead of the gateway's Squid.
- `proxy.sh`, `kms.sh`: honor `E2E_SSH_CONFIG` and `E2E_MGMT_IP_PREFIX`.
- `run-e2e.sh`: own teardown (`EXIT`/`INT`/`TERM`) after support-bundle collection and the result callback.
- Go: `ssh` dials through `proxy.FromEnvironment()` (`ALL_PROXY`); the gateway identity comes from `GATEWAY_IP` and is never parsed out of a scheme-bearing `HTTP_PROXY`; `kubectl-vsphere` receives `ALL_PROXY`.

## Constitution check

No API, controller, or webhook changes. E2E-harness only; the only `go.mod` change promotes `golang.org/x/net` to a direct use.

## Test strategy

- Ginkgo unit coverage for gateway resolution (`infrastructure/vsphere/ssh`).
- Real-testbed run of `make e2e-smoke` on an isolated VDS testbed.
