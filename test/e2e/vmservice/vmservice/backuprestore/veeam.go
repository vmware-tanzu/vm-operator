// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package backuprestore holds the backup/restore E2E suite. Unlike the
// simulated RegisterVM tests in viadmin, every backup and restore here is
// performed by a real Veeam Backup & Replication server.
package backuprestore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/veeam"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/testutils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	e2econfig "github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/skipper"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
	"github.com/vmware-tanzu/vm-operator/test/e2e/wcpframework"
)

const (
	specName = "veeam"

	// restoreMarkerAnnotation is a user annotation whose value differs
	// between the backup and the live VM, proving that RegisterVM applied
	// the backed-up VM resource.
	restoreMarkerAnnotation = "e2e.vmoperator.vmware.com/restore-marker"
	markerBeforeBackup      = "before-backup"
	markerAfterBackup       = "after-backup"
	markerAfterRestore      = "after-restore"

	// pvcProtectionFinalizer is left on the PVCs superseded by a restore to
	// an existing VM; see research.md.
	pvcProtectionFinalizer = "cns.vmware.com/pvc-protection"

	// Guest commands for the data written by the seed-data cloud-config.
	// They run as the vmware user without sudo, which some images do not
	// ship. The data disk is mounted at boot from /etc/fstab.
	cmdSeedDone     = "test -f /var/lib/vmop-seed.done && echo SEED-DONE"
	outSeedDone     = "SEED-DONE"
	cmdSeedDiverge  = "rm -f /var/lib/vmop-seed/boot.* /mnt/data/data.* && sync && echo SEED-DIVERGED"
	outSeedDiverge  = "SEED-DIVERGED"
	cmdSeedVerify   = "mountpoint -q /mnt/data && sha256sum --quiet -c /var/lib/vmop-seed.sha256 && echo SEED-VERIFIED"
	outSeedVerified = "SEED-VERIFIED"
)

// SpecInput is the input for VeeamBackupRestoreSpec.
type SpecInput struct {
	ClusterProxy     wcpframework.WCPClusterProxyInterface
	Config           *e2econfig.E2EConfig
	WCPClient        wcp.WorkloadManagementAPI
	WCPNamespaceName string
}

// VeeamBackupRestoreSpec backs up VMs with Veeam and restores them, both as a
// new VM after the original is lost and in place over an existing VM, then
// registers the result with RegisterVM. It also checks that a failed
// RegisterVM of a restored VM raises the vCenter alarm and that a later
// successful one clears it.
//
// The specs share one Veeam connection and register the testbed vCenter with
// Veeam once, before the first spec, if it is not registered yet. A vCenter
// the suite registered is removed again after the last spec.
func VeeamBackupRestoreSpec(ctx context.Context, inputGetter func() SpecInput) {
	Context("Veeam", Label("veeam"), Ordered, ContinueOnFailure, func() {
		var (
			input           SpecInput
			config          *e2econfig.E2EConfig
			svClusterClient ctrlclient.Client
			clusterProxy    *common.VMServiceClusterProxy
			t               *testEnv
		)

		BeforeAll(func() {
			input = inputGetter()
			Expect(input.Config).ToNot(BeNil(), "Invalid argument. input.Config can't be nil when calling %s spec", specName)
			Expect(input.Config.InfraConfig).ToNot(BeNil(), "Invalid argument. input.Config.InfraConfig can't be nil when calling %s spec", specName)
			skipper.SkipUnlessInfraIs(input.Config.InfraConfig.InfraName, consts.WCP)

			Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling %s spec", specName)
			Expect(input.WCPNamespaceName).ToNot(BeEmpty(), "Invalid argument. input.WCPNamespaceName can't be empty when calling %s spec", specName)

			config = input.Config
			clusterProxy = input.ClusterProxy.(*common.VMServiceClusterProxy)
			svClusterClient = clusterProxy.GetClient()

			for _, fss := range []string{"EnvFSSVMServiceBackupRestore", "EnvFSSIncrementalRestore"} {
				if !utils.IsFssEnabled(ctx, svClusterClient, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"), config.GetVariable(fss)) {
					Skip(fmt.Sprintf("%s FSS is not enabled", config.GetVariable(fss)))
				}
			}

			t = newTestEnv(ctx, input, clusterProxy)
			t.ensureVCenterRegistered(ctx)
		})

		It("Should restore a lost VM as a new VM and register it", Label("experimental"), func() {
			vmName := fmt.Sprintf("%s-new-%s", specName, capiutil.RandomString(4))

			lost := t.restoreLostVM(ctx, vmName)

			t.registerVM(ctx, lost.moID)

			vmservice.VerifyPostRegisterVM(ctx, vmName, input.WCPNamespaceName, nil, lost.diskCount, clusterProxy, config, svClusterClient, input.WCPClient)

			By("Verify the seeded data on both disks matches the backup")
			runGuestCmd(ctx, config, clusterProxy, input.WCPNamespaceName, vmName, cmdSeedVerify, outSeedVerified)

			t.verifyProtectionRestored(ctx, vmName, lost.constraints)
		})

		It("Should raise the RegisterVM alarm on failure and clear it on success", Label("experimental"), func() {
			vimClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
			defer vcenter.LogoutVimClient(vimClient)

			// Check for the alarm first so a vCenter without it does not pay for
			// a backup and restore.
			wcpAlarm := findRegisterVMAlarm(ctx, vimClient)
			if wcpAlarm == nil {
				Skip(registerVMAlarmName + " is not defined in this vCenter")
			}

			vmName := fmt.Sprintf("%s-alarm-%s", specName, capiutil.RandomString(4))

			lost := t.restoreLostVM(ctx, vmName)

			verifyRegisterVMAlarm(ctx, t, vimClient, wcpAlarm, vmName, lost.moID, lost.diskCount)
		})

		It("Should restore an existing VM in place and register it", Label("experimental"), func() {
			ns := input.WCPNamespaceName
			vmName := fmt.Sprintf("%s-existing-%s", specName, capiutil.RandomString(4))
			secretName := vmName + "-cloud-config"
			secretYaml := manifestbuilders.GetSecretYamlCloudConfigSeedData(manifestbuilders.Secret{Namespace: ns, Name: secretName})

			vm := t.createVM(ctx, vmName, secretYaml, secretName)

			By("Wait for the guest to seed data on the boot and data disks")
			vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, ns, vmName)
			runGuestCmd(ctx, config, clusterProxy, ns, vmName, cmdSeedDone, outSeedDone)

			By("Mark the VM resource and wait for the mark to reach the backup data")
			setMarker(ctx, svClusterClient, vm, markerBeforeBackup)
			waitForMarkerInBackup(ctx, config, clusterProxy, vm, markerBeforeBackup)

			vm = waitForBackupReady(ctx, config, clusterProxy, ns, vmName)
			oldVolumes := pvcNames(vm)

			// Registered after createVM's cleanups so it runs before them, and
			// deletes the overwritten PVCs itself: otherwise they are deleted
			// later and stay Terminating even when the spec fails early.
			var restoreStarted bool
			DeferCleanup(func(ctx SpecContext) {
				if restoreStarted {
					releaseOverwrittenPVCs(ctx, config, svClusterClient, ns, vmName, oldVolumes)
				}
			})

			rp := t.backupVM(ctx, vm)

			By("Diverge from the backup: delete the seeded data and change the mark")
			runGuestCmd(ctx, config, clusterProxy, ns, vmName, cmdSeedDiverge, outSeedDiverge)
			setMarker(ctx, svClusterClient, vm, markerAfterBackup)

			By("Power off and pause the VM so VM Operator does not overwrite the restored ExtraConfig")
			vmoperator.UpdateVirtualMachinePowerState(ctx, config, svClusterClient, ns, vmName, string(vmopv1.VirtualMachinePowerStateOff))
			vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, ns, vmName, string(vmopv1.VirtualMachinePowerStateOff))

			vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
			Expect(err).ToNot(HaveOccurred())

			base := vm.DeepCopy()
			metav1.SetMetaDataAnnotation(&vm.ObjectMeta, vmopv1.PauseAnnotation, "true")
			Expect(svClusterClient.Patch(ctx, vm, ctrlclient.MergeFrom(base))).To(Succeed())

			// VM Operator labels the VM once a reconcile has seen the pause, so
			// no reconcile in flight can put the constraints back after they are
			// cleared.
			By("Wait for VM Operator to pause the VM")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(vm.Labels).To(HaveKey(vmopv1.PausedVMLabelKey))
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
				"VM %s/%s was not paused", ns, vmName)

			constraints := clearExtensionCompatConstraints(ctx, clusterProxy, vm.Status.UniqueID)

			// A failed or timed-out restore may already have overwritten the
			// disks, so the cleanup releases the old PVCs from here on.
			restoreStarted = true
			t.restoreVM(ctx, rp, true, "vmop e2e restore to existing")

			By("Register the restored VM, which keeps its managed object ID")
			t.registerVM(ctx, vm.Status.UniqueID)

			By("Verify the backed-up VM resource was applied")
			Eventually(func(g Gomega) {
				vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(vm.Annotations).To(HaveKey(vmopv1.RestoredVMAnnotation))
				g.Expect(vm.Annotations).To(HaveKeyWithValue(restoreMarkerAnnotation, markerBeforeBackup))
				g.Expect(vm.Annotations).ToNot(HaveKey(vmopv1.PauseAnnotation))
				g.Expect(pvcNames(vm)).ToNot(ContainElement(BeElementOf(oldVolumes)), "VM still uses the PVCs of the overwritten disks")
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

			vmservice.VerifyPostRegisterVM(ctx, vmName, ns, nil, len(oldVolumes), clusterProxy, config, svClusterClient, input.WCPClient)

			By("Verify the seeded data on both disks matches the backup")
			runGuestCmd(ctx, config, clusterProxy, ns, vmName, cmdSeedVerify, outSeedVerified)

			t.verifyProtectionRestored(ctx, vmName, constraints)

			// The superseded PVCs point at volumes the restore destroyed. They
			// are marked for deletion but are held by the CNS PVC protection
			// finalizer, so only assert that the deletion was requested.
			By("Verify the PVCs of the overwritten VM were marked for deletion")
			for _, name := range oldVolumes {
				Eventually(func(g Gomega) {
					pvc := &corev1.PersistentVolumeClaim{}
					err := svClusterClient.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: name}, pvc)
					if apierrors.IsNotFound(err) {
						return
					}
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(pvc.DeletionTimestamp).ToNot(BeNil(), "PVC %s/%s is not marked for deletion", ns, name)
				}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
					func() string { return describePVCAndVM(ctx, svClusterClient, ns, name, vmName) })
			}
		})
	})
}

// testEnv holds the connections and settings shared by the backup/restore
// specs and their steps.
type testEnv struct {
	input        SpecInput
	config       *e2econfig.E2EConfig
	clusterProxy *common.VMServiceClusterProxy
	client       ctrlclient.Client
	vbr          *veeam.Client
	vcPNID       string
	repositoryID string
	runID        string
	linuxVMIName string
	waitOpts     veeam.WaitOptions
}

func newTestEnv(ctx context.Context, input SpecInput, clusterProxy *common.VMServiceClusterProxy) *testEnv {
	config := input.Config
	veeamCfg := config.GetVeeamConfig()

	t := &testEnv{
		input:        input,
		config:       config,
		clusterProxy: clusterProxy,
		client:       clusterProxy.GetClient(),
		vbr:          connectVeeam(ctx, veeamCfg),
		runID:        veeamCfg.RunID,
		waitOpts:     waitOptions(config),
		vcPNID:       vcenter.GetVCPNIDFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath()),
	}

	if t.runID == "" {
		t.runID = strings.ToLower(capiutil.RandomString(6))
	}

	var err error

	t.repositoryID, err = t.vbr.RepositoryID(ctx, veeamCfg.Repository)
	Expect(err).ToNot(HaveOccurred())

	linuxImageDisplayName := vmservice.GetDefaultImageDisplayName(config.InfraConfig.ManagementClusterConfig.Resources)
	t.linuxVMIName = vmoperator.WaitForVirtualMachineImageName(ctx, &config.Config, t.client, input.WCPNamespaceName, linuxImageDisplayName)

	return t
}

// ensureVCenterRegistered registers the testbed vCenter with Veeam by its
// PNID unless Veeam already manages it, so Veeam can see the test VMs. It
// removes the registration after the last spec only if it added it: the
// appliance is shared, and other runs may rely on an existing registration.
// A failed registration fails the specs rather than skipping them, because
// it means the appliance cannot reach the testbed.
func (t *testEnv) ensureVCenterRegistered(ctx context.Context) {
	server, err := t.vbr.FindManagedServer(ctx, t.vcPNID)
	Expect(err).ToNot(HaveOccurred())

	if server != nil {
		// A registration left behind by an earlier testbed with the same PNID
		// would otherwise surface later as a confusing FindVM or backup error.
		Expect(server.Status).To(Equal(veeam.ManagedServerStatusAvailable),
			"vCenter %s is registered with Veeam as managed server %s (%q) but is not available",
			t.vcPNID, server.ID, server.Description)
		framework.Logf("vCenter %s is already registered with Veeam as managed server %s", t.vcPNID, server.ID)

		return
	}

	By(fmt.Sprintf("Register vCenter %s with Veeam", t.vcPNID))

	added, err := t.vbr.RegisterVCenter(ctx, veeam.VCenterSpec{
		Name:        t.vcPNID,
		Username:    testbed.AdminUsername,
		Password:    testbed.AdminPassword,
		Description: fmt.Sprintf("Registered by the VM Operator E2E suite for vCenter %s, run %s.", t.vcPNID, t.runID),
	}, t.waitOpts)
	Expect(err).ToNot(HaveOccurred(), "failed to register vCenter %s with Veeam; the appliance must reach it on port 443 and its ESXi hosts on port 902", t.vcPNID)
	framework.Logf("Registered vCenter %s with Veeam as managed server %s", t.vcPNID, added.ID)

	DeferCleanup(func(ctx SpecContext) {
		if err := t.vbr.UnregisterVCenter(ctx, added, t.waitOpts); err != nil {
			framework.Logf("Failed to remove vCenter %s (%s) from Veeam: %v", added.Name, added.ID, err)
		}
	})
}

// createVM creates a powered-on VM with one user PVC from the given
// cloud-config Secret, waits until VM Operator has backfilled the boot disk as
// a PVC and written an up-to-date backup, and returns the VM. Everything it
// creates is deleted when the test ends.
func (t *testEnv) createVM(ctx context.Context, vmName string, secretYaml []byte, secretName string) *vmopv1.VirtualMachine {
	ns := t.input.WCPNamespaceName
	pvcName := vmName + "-data"
	resources := t.config.InfraConfig.ManagementClusterConfig.Resources

	Expect(t.clusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the cloud-config Secret")
	DeferCleanup(func(ctx SpecContext) {
		_ = t.client.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns}})
	})

	testutils.AssertCreatePVC(t.clusterProxy.GetClient(), pvcName, ns, resources.StorageClassName)
	DeferCleanup(func(ctx SpecContext) {
		_ = t.client.Delete(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: ns}})
	})

	vmYaml := manifestbuilders.GetVirtualMachineYamlA2(manifestbuilders.VirtualMachineYaml{
		Namespace:        ns,
		Name:             vmName,
		VMClassName:      resources.VMClassName,
		StorageClassName: resources.StorageClassName,
		ResourcePolicy:   resources.VMResourcePolicyName,
		ImageName:        t.linuxVMIName,
		Bootstrap: manifestbuilders.Bootstrap{
			CloudInit: &manifestbuilders.CloudInit{
				RawCloudConfig: &manifestbuilders.KeySelector{Key: "user-data", Name: secretName},
			},
		},
		PowerState: string(vmopv1.VirtualMachinePowerStateOn),
		PVCNames:   []string{pvcName},
	})
	Expect(t.clusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM:\n%s", string(vmYaml))
	DeferCleanup(func(ctx SpecContext) {
		deleteVMAndPVCs(ctx, t.client, ns, vmName)
	})

	vmoperator.WaitForVirtualMachineCreation(ctx, t.config, t.client, ns, vmName)
	vmoperator.WaitForVirtualMachineMOID(ctx, t.config, t.client, ns, vmName)
	vmoperator.WaitForPVCAttachment(ctx, t.config, t.client, ns, vmName, pvcName)
	vmoperator.WaitOnVirtualMachineCondition(ctx, t.config, t.client, ns, vmName,
		metav1.Condition{Type: consts.VMUnmanagedVolumesBackfilledCondition, Status: metav1.ConditionTrue})

	return waitForBackupReady(ctx, t.config, t.clusterProxy, ns, vmName)
}

// backupVM creates a Veeam job for the VM, runs it, and returns the newest
// restore point. The job and its backup files are deleted when the test ends.
func (t *testEnv) backupVM(ctx context.Context, vm *vmopv1.VirtualMachine) veeam.RestorePoint {
	By("Back up the VM with Veeam")

	vmRef, err := t.vbr.FindVM(ctx, t.vcPNID, vm.Name, vm.Status.UniqueID)
	Expect(err).ToNot(HaveOccurred())

	job, err := t.vbr.CreateJob(ctx, veeam.JobName(t.runID, vm.Name), t.repositoryID, vmRef)
	Expect(err).ToNot(HaveOccurred())
	framework.Logf("Created Veeam job %s (%s) for VM %s/%s", job.Name, job.ID, vm.Namespace, vm.Name)

	DeferCleanup(func(ctx SpecContext) {
		if err := t.vbr.DeleteJob(ctx, job.ID, t.waitOpts); err != nil {
			framework.Logf("Failed to clean up Veeam job %s (%s): %v", job.Name, job.ID, err)
		}
	})

	session, err := t.vbr.Backup(ctx, job.ID, t.waitOpts)
	Expect(err).ToNot(HaveOccurred())
	logSessionResult(session)

	rp, err := t.vbr.LatestRestorePoint(ctx, job.ID)
	Expect(err).ToNot(HaveOccurred())
	framework.Logf("Using Veeam restore point %s created at %s", rp.ID, rp.CreationTime)

	return rp
}

// lostVM describes a VM that restoreLostVM restored as a new vSphere VM.
type lostVM struct {
	// moID is the restored VM's managed object ID. It has no VirtualMachine
	// resource until RegisterVM adopts it.
	moID string
	// diskCount is the number of disks RegisterVM should turn into restored
	// PVCs.
	diskCount int
	// constraints is the number of extension compatibility constraints the
	// original VM had, which VM Operator should set on the restored one.
	constraints int
}

// restoreLostVM creates a VM that seeds data on its disks, backs it up with
// Veeam, deletes the VM and its user PVC, and restores it with Veeam as a new
// vSphere VM. The restored vSphere VM is destroyed when the test ends if
// RegisterVM never adopted it.
func (t *testEnv) restoreLostVM(ctx context.Context, vmName string) lostVM {
	ns := t.input.WCPNamespaceName
	secretName := vmName + "-cloud-config"
	secretYaml := manifestbuilders.GetSecretYamlCloudConfigSeedData(manifestbuilders.Secret{Namespace: ns, Name: secretName})

	vm := t.createVM(ctx, vmName, secretYaml, secretName)
	oldMoID := vm.Status.UniqueID

	By("Wait for the guest to seed data on the boot and data disks")
	vmoperator.WaitForVirtualMachineIP(ctx, t.config, t.client, ns, vmName)
	runGuestCmd(ctx, t.config, t.clusterProxy, ns, vmName, cmdSeedDone, outSeedDone)

	lost := lostVM{
		diskCount:   len(vm.Spec.Volumes),
		constraints: extensionCompatConstraintCount(ctx, t.clusterProxy, vm.Status.UniqueID),
	}

	rp := t.backupVM(ctx, vm)

	By("Lose the VM: delete the VM and its user PVC")
	vmoperator.DeleteVirtualMachine(ctx, t.client, ns, vmName)
	vmoperator.WaitForVirtualMachineToBeDeleted(ctx, t.config, t.client, ns, vmName)
	Expect(ctrlclient.IgnoreNotFound(t.client.Delete(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: vmName + "-data", Namespace: ns},
	}))).To(Succeed())

	vimClient := vcenter.NewVimClientFromKubeconfig(ctx, t.clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vimClient)

	Eventually(func(g Gomega) {
		g.Expect(findVMsByName(ctx, vimClient, vmName)).To(BeEmpty())
	}, t.config.GetIntervals("default", "wait-virtual-machine-deletion")...).Should(Succeed(),
		"vSphere VM %s was not deleted", vmName)

	// Registered after createVM's cleanups so it runs before them, while a
	// VirtualMachine resource still shows whether RegisterVM adopted the VM.
	// It looks the VM up by name, so it also catches a restore that fails or
	// times out after creating the VM.
	DeferCleanup(func(ctx SpecContext) {
		t.destroyUnregisteredVM(ctx, vmName)
	})

	t.restoreVM(ctx, rp, false, "vmop e2e restore to new")

	By("Find the restored VM, which has a new managed object ID")

	morefs, err := findVMsByName(ctx, vimClient, vmName)
	Expect(err).ToNot(HaveOccurred())
	Expect(morefs).To(HaveLen(1), "expected exactly one restored vSphere VM named %s", vmName)
	Expect(morefs[0]).ToNot(Equal(oldMoID), "a restore to new should create a new VM")

	lost.moID = morefs[0]

	return lost
}

func (t *testEnv) restoreVM(ctx context.Context, rp veeam.RestorePoint, overwrite bool, reason string) {
	By(fmt.Sprintf("Restore the VM with Veeam (overwrite: %t)", overwrite))

	session, err := t.vbr.RestoreVM(ctx, rp.ID, overwrite, reason, t.waitOpts)
	Expect(err).ToNot(HaveOccurred())
	logSessionResult(session)
}

// clearExtensionCompatConstraints removes the extension compatibility
// constraints VM Operator registered on the VM, if any, and returns how many
// it removed. The DEVICE invariant makes vCenter reject Veeam's in-place
// restore, and Veeam cannot skip the check, so a VI admin has to clear the
// constraints first. RegisterVM makes VM Operator manage the restored VM
// again, which sets them again.
func clearExtensionCompatConstraints(ctx context.Context, clusterProxy *common.VMServiceClusterProxy, moID string) int {
	vimClient := newServiceVersionVimClient(ctx, clusterProxy)
	defer vcenter.LogoutVimClient(vimClient)

	vmRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: moID}

	count, err := getExtensionCompatConstraintCount(ctx, vimClient, vmRef)
	Expect(err).ToNot(HaveOccurred())

	if count == 0 {
		return 0
	}

	By("Clear the VM's extension compatibility constraints so Veeam can restore it in place")

	// A reconfigure's constraint set replaces the whole set, so an empty set
	// clears it. Changing the constraints needs the check skipped, as they
	// protect themselves.
	task, err := object.NewVirtualMachine(vimClient, vmRef).Reconfigure(ctx, types.VirtualMachineConfigSpec{
		ExtensionCompatibilityConstraint: &types.VirtualMachineExtensionCompatibilityConstraintSet{},
		SkipExtensionCompatibilityChecks: ptr.To(true),
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(task.Wait(ctx)).To(Succeed(), "failed to clear the extension compatibility constraints of VM %s", moID)

	return count
}

// extensionCompatConstraintCount returns the number of extension
// compatibility constraints set on the VM.
func extensionCompatConstraintCount(ctx context.Context, clusterProxy *common.VMServiceClusterProxy, moID string) int {
	vimClient := newServiceVersionVimClient(ctx, clusterProxy)
	defer vcenter.LogoutVimClient(vimClient)

	count, err := getExtensionCompatConstraintCount(ctx, vimClient, types.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
	Expect(err).ToNot(HaveOccurred())

	return count
}

func getExtensionCompatConstraintCount(ctx context.Context, c *vim25.Client, vmRef types.ManagedObjectReference) (int, error) {
	var vmMO mo.VirtualMachine
	if err := property.DefaultCollector(c).RetrieveOne(ctx, vmRef, []string{"config.extensionCompatibilityConstraint"}, &vmMO); err != nil {
		return 0, err
	}

	if vmMO.Config == nil || vmMO.Config.ExtensionCompatibilityConstraint == nil {
		return 0, nil
	}

	return len(vmMO.Config.ExtensionCompatibilityConstraint.Constraint), nil
}

// newServiceVersionVimClient returns a vim client that speaks the newest API
// version vCenter serves, as VM Operator does. A development vCenter may only
// expose the extension compatibility constraints in an internal version newer
// than the release version govmomi defaults to, and answers InvalidProperty
// otherwise.
func newServiceVersionVimClient(ctx context.Context, clusterProxy *common.VMServiceClusterProxy) *vim25.Client {
	vimClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	if err := vimClient.UseServiceVersion(); err != nil {
		vcenter.LogoutVimClient(vimClient)
		Fail(fmt.Sprintf("failed to use vCenter's API version: %v", err))
	}

	return vimClient
}

// verifyProtectionRestored checks that VM Operator protects a restored VM
// again after RegisterVM: it sets the extension compatibility constraints
// again, and it writes backup data that matches the VM's current PVCs and
// tracks later changes to the VM resource, so the next backup is not stale.
func (t *testEnv) verifyProtectionRestored(ctx context.Context, vmName string, constraints int) {
	ns := t.input.WCPNamespaceName

	By("Verify VM Operator writes current backup data for the restored VM")

	vm := waitForBackupReady(ctx, t.config, t.clusterProxy, ns, vmName)
	setMarker(ctx, t.client, vm, markerAfterRestore)
	waitForMarkerInBackup(ctx, t.config, t.clusterProxy, vm, markerAfterRestore)

	By(fmt.Sprintf("Verify VM Operator set %d extension compatibility constraints on the restored VM", constraints))

	vimClient := newServiceVersionVimClient(ctx, t.clusterProxy)
	defer vcenter.LogoutVimClient(vimClient)

	vmRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}

	Eventually(func(g Gomega) {
		count, err := getExtensionCompatConstraintCount(ctx, vimClient, vmRef)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(count).To(Equal(constraints))
	}, t.config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(),
		"VM %s/%s does not have the extension compatibility constraints it had before the restore", ns, vmName)
}

// destroyUnregisteredVM destroys the vSphere VMs named vmName unless a
// VirtualMachine resource manages them, so a restored VM that RegisterVM never
// adopted does not leak with its disks.
func (t *testEnv) destroyUnregisteredVM(ctx context.Context, vmName string) {
	ns := t.input.WCPNamespaceName

	err := t.client.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: vmName}, &vmopv1.VirtualMachine{})
	if !apierrors.IsNotFound(err) {
		if err != nil {
			framework.Logf("Not destroying vSphere VM %s: failed to get VM %s/%s: %v", vmName, ns, vmName, err)
		}

		return
	}

	vimClient := vcenter.NewVimClientFromKubeconfig(ctx, t.clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vimClient)

	morefs, err := findVMsByName(ctx, vimClient, vmName)
	if err != nil {
		framework.Logf("Failed to find vSphere VM %s: %v", vmName, err)
		return
	}

	for _, moID := range morefs {
		framework.Logf("Destroying vSphere VM %s (%s), which RegisterVM did not adopt", vmName, moID)

		vm := object.NewVirtualMachine(vimClient, types.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
		if state, err := vm.PowerState(ctx); err == nil && state != types.VirtualMachinePowerStatePoweredOff {
			if task, err := vm.PowerOff(ctx); err == nil {
				_ = task.Wait(ctx)
			}
		}

		task, err := vm.Destroy(ctx)
		if err == nil {
			err = task.Wait(ctx)
		}

		if err != nil {
			framework.Logf("Failed to destroy vSphere VM %s (%s): %v", vmName, moID, err)
		}
	}
}

func (t *testEnv) registerVM(ctx context.Context, vmMoID string) {
	taskInfo, err := vmservice.InvokeRegisterVM(ctx, vmMoID, t.input.WCPNamespaceName, t.clusterProxy, t.input.WCPClient)
	Expect(err).ToNot(HaveOccurred())
	Expect(taskInfo).ToNot(BeNil())
	Expect(taskInfo.Error).To(BeNil())
	Expect(taskInfo.State).To(Equal(types.TaskInfoStateSuccess))
}

// connectVeeam returns a Veeam client. It skips the test only when no server
// is configured. A configured server that is unreachable, speaks no supported
// API version, or rejects the credentials fails the test, because that is a
// configuration problem to fix rather than a missing capability.
func connectVeeam(ctx context.Context, cfg e2econfig.VeeamConfig) *veeam.Client {
	c, err := veeam.New(ctx, veeam.Config{Server: cfg.Server, Username: cfg.Username, Password: cfg.Password})
	if err == nil {
		framework.Logf("Using Veeam server %s with REST API version %s", cfg.Server, c.APIVersion())
		return c
	}

	var connErr *veeam.ConnectError
	if errors.As(err, &connErr) && connErr.Kind == veeam.ErrorKindNotConfigured {
		Skip("no Veeam server is configured; set VEEAM_SERVER to run this test")
	}

	Fail(fmt.Sprintf("failed to connect to Veeam: %v", err))

	return nil
}

func waitOptions(config *e2econfig.E2EConfig) veeam.WaitOptions {
	parse := func(key string) (time.Duration, time.Duration) {
		intervals := config.GetIntervals("default", key)
		Expect(intervals).To(HaveLen(2), "interval %s must have a timeout and a poll interval", key)

		timeout, err := time.ParseDuration(intervals[0].(string))
		Expect(err).ToNot(HaveOccurred())

		poll, err := time.ParseDuration(intervals[1].(string))
		Expect(err).ToNot(HaveOccurred())

		return timeout, poll
	}

	timeout, poll := parse("wait-veeam-session")
	startTimeout, _ := parse("wait-veeam-session-start")

	return veeam.WaitOptions{Timeout: timeout, StartTimeout: startTimeout, Interval: poll}
}

func logSessionResult(s veeam.Session) {
	if s.Result.Result == veeam.SessionResultWarning {
		framework.Logf("Veeam session %s (%s) succeeded with a warning: %s", s.ID, s.Name, s.Result.Message)
		return
	}

	framework.Logf("Veeam session %s (%s) finished: %s", s.ID, s.Name, s.Result.Result)
}

// waitForBackupReady waits until VM Operator has written backup data for all
// of the VM's PVCs and CSI has finished registering them, so the disk chain
// Veeam snapshots is stable.
func waitForBackupReady(
	ctx context.Context,
	config *e2econfig.E2EConfig,
	clusterProxy *common.VMServiceClusterProxy,
	ns, vmName string) *vmopv1.VirtualMachine {

	vmservice.WaitForBackupToComplete(ctx, vmName, ns, clusterProxy, config, nil)
	vmoperator.WaitForVMCnsRegisterVolumesRegistered(ctx, config, clusterProxy.GetClient(), ns, vmName)

	vm, err := utils.GetVirtualMachine(ctx, clusterProxy.GetClient(), ns, vmName)
	Expect(err).ToNot(HaveOccurred())

	return vm
}

func setMarker(ctx context.Context, c ctrlclient.Client, vm *vmopv1.VirtualMachine, value string) {
	latest, err := utils.GetVirtualMachine(ctx, c, vm.Namespace, vm.Name)
	Expect(err).ToNot(HaveOccurred())

	base := latest.DeepCopy()
	metav1.SetMetaDataAnnotation(&latest.ObjectMeta, restoreMarkerAnnotation, value)
	Expect(c.Patch(ctx, latest, ctrlclient.MergeFrom(base))).To(Succeed())
}

// waitForMarkerInBackup waits until the VM resource that VM Operator stores
// in the vSphere VM's ExtraConfig carries the marker annotation, so a backup
// taken afterwards contains it.
func waitForMarkerInBackup(
	ctx context.Context,
	config *e2econfig.E2EConfig,
	clusterProxy *common.VMServiceClusterProxy,
	vm *vmopv1.VirtualMachine,
	value string) {

	vimClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vimClient)

	want := fmt.Sprintf("%s: %s", restoreMarkerAnnotation, value)
	vmRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}

	Eventually(func(g Gomega) {
		var vmMO mo.VirtualMachine
		g.Expect(property.DefaultCollector(vimClient).RetrieveOne(ctx, vmRef, []string{"config.extraConfig"}, &vmMO)).To(Succeed())
		g.Expect(vmMO.Config).ToNot(BeNil())

		var encoded string

		for _, ec := range vmMO.Config.ExtraConfig {
			if o := ec.GetOptionValue(); o.Key == backupapi.VMResourceYAMLExtraConfigKey {
				encoded, _ = o.Value.(string)
			}
		}

		g.Expect(encoded).ToNot(BeEmpty(), "VM has no %s ExtraConfig", backupapi.VMResourceYAMLExtraConfigKey)

		decoded, err := pkgutil.TryToDecodeBase64Gzip([]byte(encoded))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(decoded).To(ContainSubstring(want))
	}, config.GetIntervals("default", "wait-backup-to-complete")...).Should(Succeed(),
		"backup data of VM %s/%s does not contain %q", vm.Namespace, vm.Name, want)
}

// runGuestCmd runs cmd in the VM's guest over SSH and waits for its output
// to contain want.
func runGuestCmd(
	ctx context.Context,
	config *e2econfig.E2EConfig,
	clusterProxy *common.VMServiceClusterProxy,
	ns, vmName, cmd, want string) {

	vmIP := vmoperator.GetVirtualMachineIP(ctx, clusterProxy.GetClient(), ns, vmName)

	switch config.InfraConfig.NetworkingTopology {
	case consts.NSX:
		vmservice.WaitForPodReady(ctx, config, clusterProxy.GetClient(), ns, consts.JumpboxPodVMName)
		vmservice.VerifyLoginAndRunCmdsInNSXSetup(ctx, config, clusterProxy, ns, consts.JumpboxPodVMName, vmIP, []string{cmd}, []string{want})
	default:
		vmservice.VerifyLoginAndRunCmdsInVDSSetup(config, vmIP, []string{cmd}, []string{want})
	}
}

// findVMsByName returns the managed object IDs of all vSphere VMs with the
// given name. Test VM names carry a random suffix, so a match is the VM.
func findVMsByName(ctx context.Context, c *vim25.Client, name string) ([]string, error) {
	m := view.NewManager(c)

	v, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return nil, err
	}

	defer func() { _ = v.Destroy(ctx) }()

	var vms []mo.VirtualMachine
	if err := v.RetrieveWithFilter(ctx, []string{"VirtualMachine"}, []string{"name"}, &vms, property.Match{"name": name}); err != nil {
		// RetrieveWithFilter reports no match as an error.
		if strings.Contains(err.Error(), "object references is empty") {
			return nil, nil
		}

		return nil, err
	}

	morefs := make([]string, 0, len(vms))
	for _, vm := range vms {
		morefs = append(morefs, vm.Self.Value)
	}

	return morefs, nil
}

func pvcNames(vm *vmopv1.VirtualMachine) []string {
	var names []string

	for _, vol := range vm.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			names = append(names, vol.PersistentVolumeClaim.ClaimName)
		}
	}

	return names
}

// deleteVMAndPVCs deletes the VM and every PVC attached to it. RegisterVM
// attaches "restored-*" PVCs that are not owned by the VM, so deleting the VM
// alone would leak them into the shared namespace.
func deleteVMAndPVCs(ctx context.Context, c ctrlclient.Client, ns, vmName string) {
	vm, err := utils.GetVirtualMachine(ctx, c, ns, vmName)
	if err != nil {
		return
	}

	// A test that fails while the VM is paused would otherwise leave the
	// vSphere VM behind.
	if _, ok := vm.Annotations[vmopv1.PauseAnnotation]; ok {
		base := vm.DeepCopy()
		delete(vm.Annotations, vmopv1.PauseAnnotation)
		_ = c.Patch(ctx, vm, ctrlclient.MergeFrom(base))
	}

	_ = c.Delete(ctx, vm)

	for _, name := range pvcNames(vm) {
		_ = c.Delete(ctx, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})
	}
}

// releaseOverwrittenPVCs deletes the VM and the PVCs whose volumes a restore
// to an existing VM overwrote, and releases those PVCs, so they do not stay
// Terminating forever. Their volumes are gone, so the CNS PVC protection
// finalizer is never removed. Only call it once a restore has started. It
// waits for the VM to be gone first, so no PVC is released while a VM that a
// failed restore left untouched still uses its volume.
func releaseOverwrittenPVCs(ctx context.Context, config *e2econfig.E2EConfig, c ctrlclient.Client, ns, vmName string, names []string) {
	deleteVMAndPVCs(ctx, c, ns, vmName)

	Eventually(func() bool {
		err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: vmName}, &vmopv1.VirtualMachine{})
		return apierrors.IsNotFound(err)
	}, config.GetIntervals("default", "wait-virtual-machine-deletion")...).Should(BeTrue(),
		"VM %s/%s was not deleted", ns, vmName)

	for _, name := range names {
		pvc := &corev1.PersistentVolumeClaim{}
		if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: name}, pvc); err != nil {
			continue
		}

		if pvc.DeletionTimestamp == nil {
			if err := c.Delete(ctx, pvc); err != nil {
				framework.Logf("Failed to delete PVC %s/%s: %v", ns, name, err)
				continue
			}
		}

		base := pvc.DeepCopy()
		if controllerutil.RemoveFinalizer(pvc, pvcProtectionFinalizer) {
			if err := c.Patch(ctx, pvc, ctrlclient.MergeFrom(base)); err != nil {
				framework.Logf("Failed to remove %s from PVC %s/%s: %v", pvcProtectionFinalizer, ns, name, err)
			}
		}
	}
}

// describePVCAndVM describes a PVC and the volumes of a VM for a failure
// message.
func describePVCAndVM(ctx context.Context, c ctrlclient.Client, ns, pvcName, vmName string) string {
	var b strings.Builder

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: pvcName}, pvc); err != nil {
		fmt.Fprintf(&b, "failed to get PVC %s/%s: %v\n", ns, pvcName, err)
	} else {
		fmt.Fprintf(&b, "PVC %s/%s: phase %s, volume %s, finalizers %v, owners %v, annotations %v\n",
			ns, pvcName, pvc.Status.Phase, pvc.Spec.VolumeName, pvc.Finalizers, pvc.OwnerReferences, pvc.Annotations)
	}

	vm, err := utils.GetVirtualMachine(ctx, c, ns, vmName)
	if err != nil {
		fmt.Fprintf(&b, "failed to get VM %s/%s: %v", ns, vmName, err)
	} else {
		fmt.Fprintf(&b, "VM %s/%s PVCs: %v", ns, vmName, pvcNames(vm))
	}

	return b.String()
}
