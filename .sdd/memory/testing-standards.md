# Testing Standards

**E2E (real WCP/vSphere cluster):** Tests under `test/e2e/` follow a different layout and Makefile workflow than controller tests. Follow **e2e-sync-with-changes** to add/update/delete E2E tests with product work; use **e2e-testing** when editing `test/e2e/**/*.go`, and read **`test/e2e/README.md`** for setup, running, labels, and writing patterns.


## Test File Naming

| Suffix | Purpose | Example |
|--------|---------|---------|
| `_test.go` | All tests (unit and integration, differentiated by `Label()`) | `virtualmachinesnapshot_controller_test.go` |
| `_suite_test.go` | Ginkgo suite setup and registration | `virtualmachinesnapshot_controller_suite_test.go` |

Do **not** use the old `_unit_test.go` / `_intg_test.go` split. All tests live in `_test.go` and are differentiated exclusively by Ginkgo `Label()` decorators (see **Test Labels** below).

## Package Naming

Always use external test packages (black-box testing):

```go
package virtualmachinesnapshot_test  // Note the _test suffix
```

The external `_test` package is required because the `depguard` linter — configured in [`.golangci.yml`](../../.golangci.yml) `linters.settings.depguard` — forbids importing `testing`, `github.com/onsi/ginkgo`, `github.com/onsi/ginkgo/v2`, and `github.com/onsi/gomega` from any non-test source. An internal-package test file (`package virtualmachinesnapshot`) cannot import those packages without tripping `depguard.main`. The `depguard.test` ruleset is what allows them inside `$test` files.

## Test Labels

Use labels from `pkg/constants/testlabels` for test categorization:

```go
Describe("Reconcile",
    Label(
        testlabels.Controller,
        testlabels.API,
    ),
    unitTestsReconcile,
)
```

Integration tests additionally use `testlabels.EnvTest`.

## Suite File Pattern

Every controller test suite follows this structure:

```go
var suite = builder.NewTestSuiteForControllerWithContext(
    cource.WithContext(
        pkgcfg.NewContextWithDefaultConfig(),
    ),
    mycontroller.AddToManager,
    manager.InitializeProvidersNoopFn)

func TestMyController(t *testing.T) {
    suite.Register(t, "MyController controller suite", intgTests, unitTests)
}

var _ = BeforeSuite(func() {
    mycontroller.SkipNameValidation = ptr.To(true)
    suite.BeforeSuite()
})

var _ = AfterSuite(suite.AfterSuite)
```

Please note, it is also fine to *not* use this pattern in cases where there should be more control over things at the test-case level.

## Unit Test Pattern

Unit tests use `builder.UnitTestContextForController`:

```go
func unitTests() {
    Describe("Reconcile", Label(testlabels.Controller, testlabels.API), unitTestsReconcile)
}

func unitTestsReconcile() {
    var (
        initObjects []client.Object
        ctx         *builder.UnitTestContextForController
        reconciler  *mycontroller.Reconciler
        fakeProvider *providerfake.VMProvider
    )

    BeforeEach(func() {
        initObjects = nil
        // Set up test objects
    })

    JustBeforeEach(func() {
        ctx = suite.NewUnitTestContextForController(initObjects...)
        fakeProvider = ctx.VMProvider.(*providerfake.VMProvider)
        reconciler = mycontroller.NewReconciler(ctx, ctx.Client, ctx.Logger, ctx.Recorder, ctx.VMProvider)
    })

    AfterEach(func() {
        ctx.AfterEach()
    })

    // Test cases with When/It blocks
    When("resource exists", func() {
        It("should reconcile successfully", func() {
            // ...
        })
    })
}
```

## Integration Test Pattern

Integration tests use `builder.IntegrationTestContextForVCSim`:

```go
func intgTests() {
    Describe("Reconcile", Label(testlabels.Controller, testlabels.EnvTest, testlabels.API), intgTestsReconcile)
}

func intgTestsReconcile() {
    var (
        ctx      context.Context
        vcSimCtx *builder.IntegrationTestContextForVCSim
    )

    BeforeEach(func() {
        vcSimCtx = suite.NewIntegrationTestContextForVCSim(...)
        ctx = vcSimCtx.Context
    })

    AfterEach(func() {
        vcSimCtx.AfterEach()
    })
}
```

## Test Builders (Dummy Objects)

Use `test/builder` for creating test objects:

```go
vm := builder.DummyBasicVirtualMachine("dummy-vm", namespace)
vmSnapshot := builder.DummyVirtualMachineSnapshot(namespace, "snap-1", vm.Name)
```

## Assertions

Use Gomega dot-import style (standard in this codebase):

```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)
```

The dot import is **only** allowed inside `_test.go` files and a small set of shared test helpers — the allow-list is maintained in [`.golangci.yml`](../../.golangci.yml) `linters.exclusions.rules` (search for `should not use dot imports`). Files currently exempted include `test/builder/intg_test_context.go`, `test/builder/test_suite.go`, `test/builder/vcsim_test_context.go`, and any `_test.go` path. Anywhere else, the `revive`-style "should not use dot imports" warning is enforced.

### Async Assertions (Eventually / Consistently)

```go
// Wait for condition to become true
Eventually(func(g Gomega) {
    obj := &vmopv1.VirtualMachine{}
    g.Expect(ctx.Client.Get(ctx, objKey, obj)).To(Succeed())
    g.Expect(conditions.IsTrue(obj, vmopv1.SomeCondition)).To(BeTrue())
}).Should(Succeed())

// Verify state remains stable
Consistently(func(g Gomega) {
    // ...
}).Should(Succeed())
```

### Common Matchers

```go
Expect(err).ToNot(HaveOccurred())
Expect(err).To(MatchError(ContainSubstring("expected error")))
Expect(obj).To(BeNil())
Expect(list.Items).To(HaveLen(3))
Expect(conditions.IsTrue(obj, conditionType)).To(BeTrue())
```

## Fake Provider Setup

```go
fakeVMProvider.CreateOrUpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1.VirtualMachine) error {
    return nil  // or return specific error for error path testing
}
```

## Anti-Patterns

1. **Do NOT use `time.Sleep`** - Use `Eventually()` / `Consistently()` instead
2. **Do NOT share mutable state between tests** - Reset in `BeforeEach`
3. **Do NOT skip error checking in setup** - Use `Expect(...).To(Succeed())`
4. **Do NOT test implementation details** - Test observable behavior only
5. **Always clean up** - Use `AfterEach` with `ctx.AfterEach()`
6. **Use descriptive names** - `When("VM does not have VMware Tools", func() { ... })`
