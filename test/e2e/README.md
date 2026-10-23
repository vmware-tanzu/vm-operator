# VM Operator E2E Test Framework

This directory contains the End-to-End (E2E) test framework for VM Operator, providing comprehensive testing capabilities.

## Overview

The E2E framework validates VM Operator functionality by:
- Creating and managing VirtualMachine resources in real Kubernetes clusters
- Interacting with actual vSphere infrastructure 
- Testing feature functionality across different networking topologies (VDS/NSX)
- Validating integration with WCP components and services

## Architecture

```
test/e2e/
├── vmservice/              # Main test suite for VM Service functionality
│   ├── config/            # Test configuration files (wcp.yaml)
│   ├── vmservice/         # Core VM lifecycle and feature tests
│   │   └── virtualmachine/ # VM-specific test scenarios
│   ├── vmserviceapp/      # Application deployment tests
│   ├── lib/               # Test libraries and helpers
│   ├── common/            # Shared test utilities
│   └── consts/            # Test constants and labels
├── framework/             # Core testing framework components
├── infrastructure/        # Infrastructure interaction (vSphere, WCP)
│   └── vsphere/          # vSphere-specific clients and utilities
├── manifestbuilders/      # Test manifest generation utilities
├── utils/                 # General test utilities
├── fixtures/              # Test data and YAML fixtures
└── testutils/            # Additional testing utilities
```

## Quick Start

### Prerequisites

1. **Access to a WCP cluster** with VM Operator deployed
2. **vSphere environment** with proper permissions
3. **Kubeconfig** for the WCP supervisor cluster
4. **Go 1.26+** installed locally

### Configuration

1. **Set up the env** for your WCP cluster:
The following will export all the required environment variables
required for the test and download the kubeconfig from the Supervisor cluster.
The kubeconfig will be stored under a unique `~/.kube/<wcpIp>.kubeconfig.<random>`
file (exported as both `KUBECONFIG` and, with `--e2e`, `E2E_KUBECONFIG_PATH`,
which the E2E test reads from). The unique filename means concurrent local
runs (e.g. against two testbeds, or two runs against the same testbed in
separate terminals) won't stomp on each other's kubeconfig.

```bash
source ./hack/e2e/setup-testbed-env.sh <testbedInfo.json> --e2e
```

2. **Running Tests**

#### All E2E Tests
```bash
make test-e2e
```

Use `make test-e2e` (auto-detects), `make test-e2e-prebuilt`, or `make test-e2e-ginkgo`. The first uses `./e2e-tests` at the repo root when present (e.g. slim E2E image). Override the binary path with `E2E_PREBUILT_BINARY=/path/to/binary`.

#### Test Categories

**Smoke Tests** (quick validation):
```bash
make e2e-smoke
```

**Core Functional Tests** (comprehensive functionality):
```bash
make e2e-core
```

**Extended Functional Tests** (advanced features):
```bash
make e2e-extended
```

**Backup/Restore Tests** (real backups and restores with Veeam Backup & Replication; needs a Veeam server that can reach the testbed's vCenter on port 443 and its ESXi hosts on port 902):
```bash
VEEAM_SERVER=<vbr-host> VEEAM_USERNAME=<user> VEEAM_PASSWORD=<password> \
  make e2e-backup-restore
```

The tests are skipped only when `VEEAM_SERVER` is unset. When it is set, a server that cannot be reached, speaks no supported REST API version, or rejects the credentials fails the tests, so a misconfigured suite is visible instead of passing silently. The Veeam client never uses `HTTP_PROXY`/`HTTPS_PROXY`, which `setup-testbed-env.sh` points at the testbed gateway.

Before the first test, the suite registers the testbed vCenter with Veeam by its PNID, using the vCenter administrator credentials, unless Veeam already manages it. After the last test it removes the registration and its credentials again, but only if it added them; an existing registration is never changed. A failed registration fails the tests.

#### Focused Test Execution

**Run specific test patterns**:
```bash
make test-e2e TEST_FOCUS="VM-HARDWARE"
```

Precompiled `./e2e-tests`: pass `-e2e.*` and `--ginkgo.*` in one contiguous argument list (as `make test-e2e-prebuilt` does). Do **not** put `--` between them; otherwise `--ginkgo.focus`, `--ginkgo.label-filter`, etc. are ignored and nearly all specs still run.

**Skip specific tests**:
```bash
make test-e2e TEST_SKIP="encryption|snapshot"
```

**Label-based filtering**:
```bash
make test-e2e LABEL_FILTER="networking && !encryption"
```

**Flake retry**:
```bash
make test-e2e FLAKE_ATTEMPTS=3
```

**Use specific namespace**:
```bash
E2E_NAMESPACE=my-test-ns make e2e-smoke
```

**Combine multiple options**:
```bash
E2E_NAMESPACE=debug-ns TEST_FOCUS="VM-HARDWARE" FLAKE_ATTEMPTS=2 make e2e-core
```

## Test Categories and Labels

Tests are organized using Ginkgo labels for easy filtering:

### Core Labels
- `smoke` - Quick validation tests
- `core-functional` - Core feature tests
- `extended-functional` - Advanced feature tests
- `experimental` - Excluded from all CI targets; requires infrastructure unavailable in the standard `vds_standard_medium` testbed (e.g. encryption-capable storage)
- `backup-restore` - Backup/restore tests that need an external backup server. Run only by `make e2e-backup-restore` (and the matching CI suite); excluded from the smoke, core, and extended targets. While these tests also carry `experimental`, the `backup-restore` filter still selects them so they can be validated in their own suite.
- `veeam` - The `backup-restore` tests that use Veeam Backup & Replication. They run in order and share one Veeam connection and vCenter registration.
- `boot-disk-capacity` - The tests for `spec.advanced.bootDiskCapacity`. Run them with `make test-e2e LABEL_FILTER="boot-disk-capacity"`.

## Configuration

### Main Configuration File

The primary configuration is in `vmservice/config/wcp.yaml`:

### Environment Variables

| Variable | Description | Default | Makefile Support |
|----------|-------------|---------|------------------|
| `NETWORK` | Networking topology (vds/nsx) | `vds` | ❌ |
| `STORAGE_CLASS` | Storage class for VMs | `wcpglobal-storage-profile` | ❌ |
| `E2E_KUBECONFIG_PATH` | Supervisor kubeconfig path (set by `setup-testbed-env.sh --e2e`; a unique file per invocation avoids concurrent local runs stomping on each other) | `~/.kube/wcp-config` | ❌ |
| `E2E_NAMESPACE` | Fixed namespace for tests | Random namespace | ✅ |
| `TEST_FOCUS` | Ginkgo focus pattern | All tests | ✅ |
| `TEST_SKIP` | Ginkgo skip pattern | No skips | ✅ |
| `LABEL_FILTER` | Ginkgo label filter | No filter | ✅ |
| `FLAKE_ATTEMPTS` | Retry count for flaky tests | No retries | ✅ |
| `VEEAM_SERVER` | Veeam B&R REST API: `host`, `host:port`, or `https://host:port` (port defaults to `9419`) | Unset (backup/restore tests skip) | ❌ |
| `VEEAM_USERNAME` / `VEEAM_PASSWORD` | Veeam B&R credentials | Unset | ❌ |
| `VEEAM_REPOSITORY` | Name of the backup repository the tests' jobs write to | First repository | ❌ |
| `E2E_RUN_ID` | ID folded into the names of Veeam jobs (`vmop-e2e-<run-id>-<vm>`) so runs sharing a server don't collide | Random | ❌ |

#### Namespace Management

The `E2E_NAMESPACE` variable controls which Kubernetes namespace the tests use. It's automatically passed through all Makefile targets to the test configuration.

**Default behavior** (random namespace):
```bash
make e2e-smoke  # Creates random namespace like "vmservice-e2e-abc123"
```

**Fixed namespace** (reuse existing namespace):
```bash
make e2e-smoke E2E_NAMESPACE=my-test-ns  # Uses "my-test-ns" namespace
```

**Benefits of fixed namespace**:
- Faster test execution (no namespace creation/deletion)
- Easier debugging 
- Shared test environment setup
- Consistent test isolation

## Writing New Tests

### Test Structure

Follow the established patterns in `vmservice/vmservice/virtualmachine/`:

```go
var _ = Describe("My Feature", Label("my-feature"), func() {
    var (
        ctx          context.Context
        vm           *vmopv1.VirtualMachine
        vmKey        types.NamespacedName
    )

    BeforeEach(func() {
        // Setup test environment
        vm = &vmopv1.VirtualMachine{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "test-vm",
                Namespace: namespace,
            },
            Spec: vmopv1.VirtualMachineSpec{
                // VM specification
            },
        }
    })

    When("feature is enabled", func() {
        It("should work correctly", func() {
            // Test implementation
            vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm, input.SkipCleanup)
            
            Eventually(func(g Gomega) {
                g.Expect(client.Get(ctx, vmKey, vm)).To(Succeed())
                g.Expect(vm.Status.Phase).To(Equal(vmopv1.Created))
            }).Should(Succeed())
        })
    })
})
```

### Best Practices

1. **Use descriptive test names** that explain the scenario
2. **Apply appropriate labels** for filtering
3. **Use Eventually/Consistently** for async operations
4. **Register cleanup when creating resources** with `DeferCleanup` helpers
5. **Test both positive and negative cases**
6. **Use manifest builders** for complex objects
7. **Validate status conditions** and events

### Helper Libraries

#### VM Operations (`lib/vmoperator`)

Prefer a direct `vmopv1.VirtualMachine` literal for ordinary VM fixtures. Use `CreateVirtualMachine` to create it and immediately register failure diagnostics and deferred cleanup. Pass the same cluster proxy used by the test's kubectl operations: its client loads that proxy's kubeconfig and current context, without overriding the Kubernetes user. When refactoring an existing fixture, preserve its API version using the corresponding versioned Go type.

```go
vm := &vmopv1.VirtualMachine{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "test-vm",
        Namespace: namespace,
    },
    Spec: vmopv1.VirtualMachineSpec{
        ClassName:    "best-effort-small",
        ImageName:    imageName,
        StorageClass: storageClassName,
        PowerState:   vmopv1.VirtualMachinePowerStateOn,
    },
}
vmoperator.CreateVirtualMachine(ctx, config, clusterProxy, vm, input.SkipCleanup)
vmoperator.WaitForVirtualMachineCreation(ctx, config, clusterProxy.GetClient(), vm.Namespace, vm.Name)
```

#### Manifest Builders (`manifestbuilders`)

Avoid introducing `VirtualMachineYaml` just to rename API fields, and set bootstrap configuration with typed API literals rather than inline YAML. The typed `VirtualMachineA5` constructor builds only the VM; build its PVCs with `PersistentVolumeClaims` and create them separately. Use `manifestbuilders.ToYAML` when a test specifically needs a YAML document, such as a kubectl validation test.

#### Infrastructure Access (`infrastructure/vsphere`)
```go
vcClient := vcenter.NewVimClientFromKubeconfig(ctx, kubeconfigPath)
```

### Debug Mode

**Enable verbose output**:
```bash
make test-e2e GINKGO_ARGS="-v --trace"
```

**Test-specific debugging**:
```go
By("Debug checkpoint: VM creation")
framework.Logf("VM status: %+v", vm.Status)
```

### Encryption Key Provider Setup (`hack/e2e/kms.sh`)

`setup-testbed-env.sh --e2e` runs `hack/e2e/kms.sh` to register the `gce2e-native` and `gce2e-standard` key providers in vCenter. `gce2e-standard` needs an external PyKMIP server: the testbed's dedicated PyKMIP server if there is one, otherwise the gateway VM. `kms.sh install` configures both providers; `kms.sh setup` configures only `gce2e-native`, and is used when no PyKMIP server can be set up.

If the last attempt fails, `kms.sh` exits non-zero and `setup-testbed-env.sh` fails too and therefore stops before any test runs, including the tests that do not use encryption. The provider is left registered so that it can be inspected.

Parameters:

| Variable | Description | Default |
|----------|-------------|---------|
| `KMS_MAX_ATTEMPTS` | Total number of deployment attempts | `4` |
| `KMS_GREEN_TIMEOUT` | Seconds to wait for `gce2e-standard` to become green after each attempt | `120` |
| `KMS_RETRY_DELAY` | Seconds to wait after the first failed attempt before the next one. The wait doubles after each further failure, so the default of four attempts waits 30s, 60s, and then 120s | `30` |
| `SSH_CONNECT_TIMEOUT` | Seconds to wait for each SSH connection to the PyKMIP host or gateway VM | `30` |
| `PIP_INDEX_URL` | It must be a full index URL including the scheme, such as `https://packages.vcfd.broadcom.net/artifactory/api/pypi/photonos-pypi-virtual/simple` | Unset (public PyPI) |


## Development Workflow

1. **Write tests** following established patterns
2. **Add appropriate labels** for categorization
3. **Test locally** with `make test-e2e TEST_FOCUS="MyFeature"`
4. **Clean up artifacts** with `make clean` before Docker builds
5. **Update documentation** for new test categories

### Cleanup and Optimization

**Remove build artifacts** before Docker operations:
```bash
make clean  # Removes *.test binaries, coverage files, and build artifacts
```

## Integration Points

- **VM Operator Controllers** - Tests controller reconciliation
- **vSphere APIs** - Validates infrastructure integration  
- **WCP Services** - Tests platform service interactions
- **Kubernetes APIs** - Validates CRD and webhook behavior
- **Feature Gates** - Tests feature flag functionality

This E2E framework ensures VM Operator reliability across diverse environments and use cases, providing confidence in production deployments.
