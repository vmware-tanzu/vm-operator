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
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/veeam"
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

	// pvcProtectionFinalizer is left on the PVCs superseded by a restore to
	// an existing VM; see research.md.
	pvcProtectionFinalizer = "cns.vmware.com/pvc-protection"

	// Guest commands for the data written by the seed-data cloud-config.
	cmdSeedDone     = "sudo test -f /root/seed.done && echo SEED-DONE"
	outSeedDone     = "SEED-DONE"
	cmdSeedDiverge  = "sudo rm -f /var/lib/vmop-seed/boot.* /mnt/data/data.* && sync && echo SEED-DIVERGED"
	outSeedDiverge  = "SEED-DIVERGED"
	cmdSeedVerify   = "sudo mount -a && sudo sha256sum --quiet -c /root/seed.sha256 && echo SEED-VERIFIED"
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
func VeeamBackupRestoreSpec(ctx context.Context, inputGetter func() SpecInput) {
	var (
		input           SpecInput
		config          *e2econfig.E2EConfig
		svClusterClient ctrlclient.Client
		clusterProxy    *common.VMServiceClusterProxy
		t               *testEnv
	)

	BeforeEach(func() {
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
	})

	It("Should restore a lost VM as a new VM and register it", Label("experimental"), func() {
		vmName := fmt.Sprintf("%s-new-%s", specName, capiutil.RandomString(4))

		moID, diskCount := t.restoreLostVM(ctx, vmName)

		t.registerVM(ctx, moID)

		vmservice.VerifyPostRegisterVM(ctx, vmName, input.WCPNamespaceName, nil, diskCount, clusterProxy, config, svClusterClient, input.WCPClient)
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

		moID, diskCount := t.restoreLostVM(ctx, vmName)

		verifyRegisterVMAlarm(ctx, t, vimClient, wcpAlarm, vmName, moID, diskCount)
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
		}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())

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
			}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed())
		}

		DeferCleanup(func(ctx SpecContext) {
			removePVCProtectionFinalizers(ctx, svClusterClient, ns, oldVolumes)
		})

		vmservice.VerifyPostRegisterVM(ctx, vmName, ns, nil, len(oldVolumes), clusterProxy, config, svClusterClient, input.WCPClient)

		By("Verify the seeded data on both disks matches the backup")
		runGuestCmd(ctx, config, clusterProxy, ns, vmName, cmdSeedVerify, outSeedVerified)
	})
}

// testEnv holds the per-test connections and settings shared by the steps of
// a backup/restore test.
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

	testutils.AssertCreatePVC(t.clusterProxy.GetClientSet(), pvcName, ns, resources.StorageClassName)
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

// restoreLostVM creates a VM, backs it up with Veeam, deletes the VM and its
// user PVC, and restores it with Veeam as a new vSphere VM. It returns the
// restored VM's managed object ID, which has no VirtualMachine resource yet,
// and the number of disks RegisterVM should turn into restored PVCs.
func (t *testEnv) restoreLostVM(ctx context.Context, vmName string) (string, int) {
	ns := t.input.WCPNamespaceName
	secretName := vmName + "-cloud-config"
	secretYaml := manifestbuilders.GetSecretYamlCloudConfig(manifestbuilders.Secret{Namespace: ns, Name: secretName})

	vm := t.createVM(ctx, vmName, secretYaml, secretName)
	oldMoID := vm.Status.UniqueID
	diskCount := len(vm.Spec.Volumes)

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

	t.restoreVM(ctx, rp, false, "vmop e2e restore to new")

	By("Find the restored VM, which has a new managed object ID")

	morefs, err := findVMsByName(ctx, vimClient, vmName)
	Expect(err).ToNot(HaveOccurred())
	Expect(morefs).To(HaveLen(1), "expected exactly one restored vSphere VM named %s", vmName)
	Expect(morefs[0]).ToNot(Equal(oldMoID), "a restore to new should create a new VM")

	return morefs[0], diskCount
}

func (t *testEnv) restoreVM(ctx context.Context, rp veeam.RestorePoint, overwrite bool, reason string) {
	By(fmt.Sprintf("Restore the VM with Veeam (overwrite: %t)", overwrite))

	session, err := t.vbr.RestoreVM(ctx, rp.ID, overwrite, reason, t.waitOpts)
	Expect(err).ToNot(HaveOccurred())
	logSessionResult(session)
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

	vmservice.WaitForBackupToComplete(ctx, vmName, ns, clusterProxy, config)
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

// removePVCProtectionFinalizers releases PVCs whose volumes a restore to an
// existing VM destroyed, so they do not stay Terminating forever.
func removePVCProtectionFinalizers(ctx context.Context, c ctrlclient.Client, ns string, names []string) {
	for _, name := range names {
		pvc := &corev1.PersistentVolumeClaim{}
		if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: name}, pvc); err != nil || pvc.DeletionTimestamp == nil {
			continue
		}

		base := pvc.DeepCopy()
		if controllerutil.RemoveFinalizer(pvc, pvcProtectionFinalizer) {
			if err := c.Patch(ctx, pvc, ctrlclient.MergeFrom(base)); err != nil {
				framework.Logf("Failed to remove %s from PVC %s/%s: %v", pvcProtectionFinalizer, ns, name, err)
			}
		}
	}
}
