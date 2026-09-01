// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/cource"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// Shared by vsphere_test VM specs (vmprovider_vm_*_test.go, fast deploy, resize, …).
const (
	cvmiKind     = "ClusterVirtualMachineImage"
	vcsimCPUFreq = 2294
	dvpgName     = "DC0_DVPG0"
)

const (
	createOrUpdateVMMaxAllowedCallCount = 100
)

func createOrUpdateVM(
	testCtx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) error {

	var fn func(ctx context.Context) error

	if pkgcfg.FromContext(testCtx).AsyncSignalEnabled &&
		pkgcfg.FromContext(testCtx).AsyncCreateEnabled {

		By("non-blocking createOrUpdateVM")
		fn = func(ctx context.Context) error {
			return createOrUpdateVMAsync(testCtx, provider, vm)
		}
	} else {
		By("blocking createOrUpdateVM")
		fn = func(ctx context.Context) error {
			return provider.CreateOrUpdateVirtualMachine(ctx, vm)
		}
	}

	var (
		totalCallCount    = 0
		nonErrorCallCount = 0
	)

	for {
		var (
			err    error
			repeat bool
			opctx  = ctxop.WithContext(testCtx)
		)

		err = fn(opctx)

		if ctxop.IsUpdate(opctx) {
			ctxop.MarkUpdate(testCtx)
		}

		if err != nil {
			switch {
			case errors.Is(err, vsphere.ErrCreate),
				errors.Is(err, vsphere.ErrBackup),
				errors.Is(err, vsphere.ErrBootstrapCustomize),
				errors.Is(err, vsphere.ErrBootstrapReconfigure),
				errors.Is(err, vsphere.ErrReconfigure),
				errors.Is(err, vsphere.ErrRestart),
				errors.Is(err, vsphere.ErrSetPowerState),
				errors.Is(err, vsphere.ErrUpgradeHardwareVersion),
				errors.Is(err, vsphere.ErrPromoteDisks),
				errors.Is(err, vsphere.ErrSnapshotRevert),
				errors.Is(err, vsphere.ErrPolicyNotReady),
				errors.Is(err, vsphere.ErrUpgradeSchema),
				errors.Is(err, vsphere.ErrUpgradeObject):

				repeat = true
			default:
				GinkgoLogr.Error(err, "createOrUpdateVM fail")
				return err
			}
		}

		if totalCallCount > 100 {
			ExpectWithOffset(1, totalCallCount).To(
				BeNumerically("<", createOrUpdateVMMaxAllowedCallCount),
				"cannot exceed createOrUpdateVMMaxAllowedCallCount for tests")
		}

		totalCallCount++

		if !repeat {
			nonErrorCallCount++
		}

		if nonErrorCallCount == 2 {
			GinkgoLogr.Info(
				"createOrUpdateVM success",
				"totalCalls", totalCallCount)
			return nil
		}

		GinkgoLogr.Info(
			"createOrUpdateVM repeat",
			"totalCalls", totalCallCount,
			"err", err)
	}
}

func createOrUpdateAndGetVcVM(
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) (*object.VirtualMachine, error) {

	if err := createOrUpdateVM(ctx, provider, vm); err != nil {
		return nil, err
	}

	ExpectWithOffset(1, vm.Status.UniqueID).ToNot(BeEmpty())
	vcVM := ctx.GetVMFromMoID(vm.Status.UniqueID)
	ExpectWithOffset(1, vcVM).ToNot(BeNil())
	return vcVM, nil
}

func createOrUpdateVMAsync(
	ctx *builder.TestContextForVCSim,
	provider providers.VirtualMachineProviderInterface,
	vm *vmopv1.VirtualMachine) error {

	GinkgoLogr.Info("entered createOrUpdateVMAsync")

	chanErr, err := provider.CreateOrUpdateVirtualMachineAsync(ctx, vm)
	if err != nil {
		if errors.Is(err, vsphere.ErrUpgradeSchema) ||
			errors.Is(err, vsphere.ErrUpgradeObject) {

			ExpectWithOffset(1, ctx.Client.Update(
				ctx,
				vm)).To(Succeed())
		}
		GinkgoLogr.Info("createOrUpdateVMAsync returned", "err", err)
		return err
	}

	if chanErr != nil {
		// Unlike the VM controller, this test helper blocks until the async
		// parts of CreateOrUpdateVM are complete. This is to avoid a large
		// refactor for now.
		for err2 := range chanErr {
			if err2 != nil {
				GinkgoLogr.Info("createOrUpdateVMAsync chanErr", "err", err2)
				if err == nil {
					err = err2
				} else {
					err = fmt.Errorf("%w,%w", err, err2)
				}
			}
		}
	}

	if errors.Is(err, vsphere.ErrCreate) {
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			client.ObjectKeyFromObject(vm),
			vm)).To(Succeed())
	}

	GinkgoLogr.Info("createOrUpdateVMAsync returned post channel", "err", err)
	return err
}

// The helpers below implement the setup shared by the VM specs in
// vmprovider_vm_*_test.go. A spec declares its own closure variables and Ginkgo
// nodes so that the shape of its setup stays visible, but the bodies of those
// nodes delegate here:
//
//	BeforeEach(func() {
//		parentCtx = newVMTestParentContext()
//		testConfig = newVMTestConfig()
//		vmClass, vm = newVMTestObjects("test-vm")
//		initObjects = nil
//	})
//
//	JustBeforeEach(func() {
//		ctx, vmProvider, nsInfo = setupVMTest(
//			parentCtx, testConfig, vmClass, vm, initObjects...)
//	})
//
//	AfterEach(func() {
//		vmTestAfterEach(ctx, vm)
//		ctx, vmProvider, vm, vmClass = nil, nil, nil, nil
//		nsInfo = builder.WorkloadNamespaceInfo{}
//	})
//
// Specs that need to interleave their own work — registering a vmconfig
// reconciler, patching the VM class ConfigSpec, installing a vcsim handler —
// call the finer-grained helpers (newVMTestContext, resolveVMTestImage,
// createVMTestObjects) instead of setupVMTest.

// newVMTestParentContext returns the parent context shared by the VM specs. It
// is built in a BeforeEach rather than a JustBeforeEach so that a spec may
// adjust the config or register vmconfig reconcilers on it before the vcsim
// context is created.
//
// Fast Deploy is on, because that is how VM Operator deploys a VM on a modern
// Supervisor; a spec that must exercise the legacy content library deploy path
// calls disableFastDeploy.
func newVMTestParentContext() context.Context {
	parentCtx := pkgcfg.NewContextWithDefaultConfig()
	parentCtx = ctxop.WithContext(parentCtx)
	parentCtx = ovfcache.WithContext(parentCtx)
	parentCtx = cource.WithContext(parentCtx)
	pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
		config.AsyncCreateEnabled = false
		config.AsyncSignalEnabled = false
		config.Features.FastDeploy = true
	})
	return parentCtx
}

// newVMTestConfig returns the vcsim configuration shared by the VM specs.
func newVMTestConfig() builder.VCSimTestConfig {
	return builder.VCSimTestConfig{
		WithContentLibrary: true,
	}
}

// newVMTestObjects returns the VM class and VM shared by the VM specs. The VM's
// network is disabled so that a spec only pays for networking when it is
// actually part of the behavior under test.
func newVMTestObjects(
	vmName string) (*vmopv1.VirtualMachineClass, *vmopv1.VirtualMachine) {

	vmClass := builder.DummyVirtualMachineClassGenName()
	vm := builder.DummyBasicVirtualMachine(vmName, "")

	if vm.Spec.Network == nil {
		vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
	}
	vm.Spec.Network.Disabled = true

	return vmClass, vm
}

// newVMTestContext creates the vcsim test context and VM provider shared by the
// VM specs, along with a workload namespace to deploy into.
func newVMTestContext(
	parentCtx context.Context,
	testConfig builder.VCSimTestConfig,
	initObjects ...client.Object) (
	*builder.TestContextForVCSim,
	providers.VirtualMachineProviderInterface,
	builder.WorkloadNamespaceInfo) {

	ctx := suite.NewTestContextForVCSimWithParentContext(parentCtx, testConfig, initObjects...)
	pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
		config.MaxDeployThreadsOnProvider = 1
	})
	vmProvider := vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	nsInfo := ctx.CreateWorkloadNamespace()

	return ctx, vmProvider, nsInfo
}

// resolveVMTestImage returns the image the VM specs deploy from. When the test
// context has a content library, the image backed by its first library item is
// used. Otherwise a VM from the vcsim inventory is used as the clone source,
// which requires bypassing the content library provider check.
func resolveVMTestImage(
	ctx *builder.TestContextForVCSim,
	testConfig builder.VCSimTestConfig) *vmopv1.ClusterVirtualMachineImage {

	img := &vmopv1.ClusterVirtualMachineImage{}

	if testConfig.WithContentLibrary {
		ExpectWithOffset(1, ctx.Client.Get(
			ctx,
			client.ObjectKey{Name: ctx.ContentLibraryItem1Name},
			img)).To(Succeed())

		return img
	}

	// BMV: VM creation without a content library is broken - and has been for a
	// long while - since we assume the VM image will always point to a content
	// library item. Hack around that with this knob so we can continue to test
	// the VM clone path.
	vsphere.SkipVMImageCLProviderCheck = true

	img = builder.DummyClusterVirtualMachineImage("DC0_C0_RP0_VM0")
	ExpectWithOffset(1, ctx.Client.Create(ctx, img)).To(Succeed())
	conditions.MarkTrue(img, vmopv1.ReadyConditionType)
	ExpectWithOffset(1, ctx.Client.Status().Update(ctx, img)).To(Succeed())

	return img
}

// createVMTestObjects creates the VM class and the VM in the workload
// namespace, pointing the VM at the provided class and image and at the test
// context's storage class.
func createVMTestObjects(
	ctx *builder.TestContextForVCSim,
	nsInfo builder.WorkloadNamespaceInfo,
	img *vmopv1.ClusterVirtualMachineImage,
	vmClass *vmopv1.VirtualMachineClass,
	vm *vmopv1.VirtualMachine) {

	var className string
	if vmClass != nil {
		vmClass.Namespace = nsInfo.Namespace
		ExpectWithOffset(1, ctx.Client.Create(ctx, vmClass)).To(Succeed())
		className = vmClass.Name
	}

	vm.Namespace = nsInfo.Namespace
	vm.Spec.ClassName = className
	vm.Spec.ImageName = img.Name
	vm.Spec.Image.Kind = cvmiKind
	vm.Spec.Image.Name = img.Name
	vm.Spec.StorageClass = ctx.StorageClassName

	ExpectWithOffset(1, ctx.Client.Create(ctx, vm)).To(Succeed())
}

// setupVMTest performs the whole JustBeforeEach shared by the VM specs: it
// creates the vcsim test context, VM provider, and workload namespace, then
// creates the VM class and VM in that namespace. When Fast Deploy is enabled it
// also reports the image's files as cached, since nothing else in a provider
// test plays the part of the image cache controller.
func setupVMTest(
	parentCtx context.Context,
	testConfig builder.VCSimTestConfig,
	vmClass *vmopv1.VirtualMachineClass,
	vm *vmopv1.VirtualMachine,
	initObjects ...client.Object) (
	*builder.TestContextForVCSim,
	providers.VirtualMachineProviderInterface,
	builder.WorkloadNamespaceInfo) {

	ctx, vmProvider, nsInfo := newVMTestContext(parentCtx, testConfig, initObjects...)

	img := resolveVMTestImage(ctx, testConfig)

	if testConfig.WithContentLibrary && pkgcfg.FromContext(ctx).Features.FastDeploy {
		ctx.MarkImageCacheReady(ctx.ContentLibraryItem1Cache)
	}

	createVMTestObjects(ctx, nsInfo, img, vmClass, vm)

	return ctx, vmProvider, nsInfo
}

// disableFastDeploy turns Fast Deploy off for a spec that must exercise the
// legacy content library deploy path — instance storage, for instance, which
// Fast Deploy does not support. See
// pkg/providers/vsphere/placement/zone_placement.go.
func disableFastDeploy(parentCtx context.Context) {
	pkgcfg.SetContext(parentCtx, func(config *pkgcfg.Config) {
		config.Features.FastDeploy = false
	})
}

// pinVMToFirstZone places the VM into the first zone the test context created
// by way of the topology label. It returns the zone's name and must be called
// after the VM has been created.
func pinVMToFirstZone(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine) string {

	zoneName := ctx.GetFirstZoneName()
	vm.Labels[corev1.LabelTopologyZone] = zoneName
	ExpectWithOffset(1, ctx.Client.Update(ctx, vm)).To(Succeed())

	return zoneName
}

// vmTestAfterEach asserts the invariants shared by the VM specs and tears down
// the test context.
func vmTestAfterEach(
	ctx *builder.TestContextForVCSim,
	vm *vmopv1.VirtualMachine) {

	vsphere.SkipVMImageCLProviderCheck = false

	if vm != nil &&
		!pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {

		By("Assert vm.Status.Crypto is nil when BYOK is disabled", func() {
			ExpectWithOffset(1, vm.Status.Crypto).To(BeNil())
		})
	}

	ctx.AfterEach()
}
