# Tasks: E2E Support for Isolated Testbeds

- [x] T001 [vmop-4430] Add `hack/e2e/isolated.sh` (jumphost discovery, tunnel lifecycle, ssh_config)
- [x] T002 [vmop-4430] Wire isolated mode into `setup-testbed-env.sh`, `proxy.sh`, `kms.sh`
- [x] T003 [vmop-4430] Tunnel teardown in `run-e2e.sh`
- [x] T004 [vmop-4430] Go SSH dialer honors `ALL_PROXY`; gateway identity decoupled from `HTTP_PROXY`
- [x] T005 [vmop-4430] Unit tests for gateway resolution; document in `test/e2e/README.md`
- [x] T006 [vmop-4430] Validate `make e2e-smoke` on an isolated VDS testbed
- [x] T007a [vmop-4430] Validate KMIP on isolated VDS with `PIP_INDEX_URL` set to the corporate PyPI mirror
- [ ] T007b [vmop-4430] Validate isolated NSX/VPC and Packer
- [x] T008 [vmop-4430] Kubernetes streaming: SOCKS is unsupported, add an HTTP CONNECT proxy via an SSH local forward to the gateway Squid
- [ ] T009 [vmop-4430] Container integration: UTS pipeline wiring and a real `deliverable_blob` input
- [x] T010 [vmop-4430] Supervise the jumphost master connection and reconnect if it drops
