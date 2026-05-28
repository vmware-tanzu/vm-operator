# E2E Tests (`test/e2e`)

**Product + E2E together:** `e2e-sync-with-changes.md` (always on) — add, update, or delete E2E tests alongside related code changes.

**Canonical documentation:** `test/e2e/README.md` — prerequisites, architecture, env vars, labels, debugging, and writing patterns. Prefer updating that README when workflows change; keep this rule as a short pointer plus the commands used most often.

## Running tests

**Environment setup** (kubeconfig under `~/.kube/wcp-config`, vars exported — see README for `testbedInfo.json` or a remote URL):

```bash
source ./hack/e2e/setup-testbed-env.sh <testbedInfo.json|URL> --e2e
```

**Makefile targets** (from repo root):

| Goal | Command |
|------|---------|
| Full E2E | `make test-e2e` |
| Smoke | `make e2e-smoke` |
| Core | `make e2e-core` |
| Extended | `make e2e-extended` |

**Prebuilt vs compile:** `make test-e2e` auto-detects `./e2e-tests` vs `test-e2e-ginkgo`. Prebuilt binaries need `-e2e.*` and `--ginkgo.*` in **one** argv block (no `--` separator); splitting with `--` drops Ginkgo filters.

**Common filters** (passed through Makefile — see README for full list):

```bash
make test-e2e TEST_FOCUS="PATTERN"
make test-e2e TEST_SKIP="a|b"
make test-e2e LABEL_FILTER="smoke && !encryption"
make test-e2e FLAKE_ATTEMPTS=3
E2E_NAMESPACE=my-ns make e2e-smoke
make test-e2e GINKGO_ARGS="-v --trace"
```

Config file: `test/e2e/vmservice/config/wcp.yaml`. Env vars such as `NETWORK`, `STORAGE_CLASS`, `E2E_NAMESPACE`, `TEST_FOCUS`, etc. are documented in the README table.

## Writing tests

- Follow layouts and examples under `test/e2e/vmservice/` (e.g. `vmservice/virtualmachine/`).
- Use Ginkgo `Label(...)` with the project’s label scheme (`smoke`, `core-functional`, `extended-functional`, feature labels — see README).
- Prefer helpers in `lib/vmoperator`, `manifestbuilders`, and `infrastructure/vsphere` rather than ad-hoc API calls.
- Async behavior: `Eventually` / `Consistently`, not sleeps — same spirit as controller tests; details and snippets are in the README.

### Linter scope for E2E

The `test/e2e/` tree is deliberately exempt from most of the project's lint rules. [`.golangci.yml`](../../.golangci.yml) `linters.exclusions.rules` includes a `- path: "test/e2e"` block that disables `revive`, `gosec`, `prealloc`, `wsl` / `wsl_v5`, `gocritic`, `gocyclo`, `noinlineerr`, `makezero`, `modernize`, `gocheckcompilerdirectives`, `embeddedstructcheck`, **`importas`**, `ineffassign`, `goconst`, **`depguard`**, `godoclint`, and `staticcheck` inside that path. In practice this means:

- The `importas` table that controls package aliases in the rest of the repo (see [`architectural-standards.md`](./architectural-standards.md) "Package Alias Conventions") is **not** enforced under `test/e2e/`. Pick aliases that match the surrounding E2E code rather than the controller-side conventions.
- E2E test files may use dot imports for ginkgo / gomega without tripping any rule.
- `depguard` is off, so E2E code may import packages (e.g. `k8s.io/utils`) that are forbidden elsewhere — keep usage minimal and intentional, and prefer the project's existing helpers in `lib/vmoperator` / `manifestbuilders`.

### Structure and manifests

- **Avoid inline closures** (`func(...) { ... }` assigned to a variable) inside `Describe` / `Context` unless they are clearly the smallest way to express a one-off and would not read cleaner as a few lines at the call site or a **package-level** helper in the same test package. Prefer inlining the steps in `It` / `BeforeEach`, or extending **`manifestbuilders`** / **`createPvcsFromSpec`**-style helpers so PVC and VM shapes stay consistent with the rest of the suite.
- **PVCs**: Build volume claims with `createPvcsFromSpec` (or the same `manifestbuilders.PVC` fields it fills) and **`manifestbuilders.GetPersistentVolumeClaimYaml`** when you need a **standalone** PVC document (e.g. apply the PVC before the VM so admission can resolve the claim). That keeps YAML aligned with `GetVirtualMachineYamlA5` / `v1a5singlevm.yaml.in`.
- **Apply path**: If `controller-runtime` `Create` on a PVC fails against CSI mutation webhooks (e.g. TLS timeout), use **`clusterProxy.ApplyWithArgs`** with the rendered YAML so the same path as `kubectl apply` is used.

When adding new categories, labels, or Makefile variables, **update `test/e2e/README.md`**.
