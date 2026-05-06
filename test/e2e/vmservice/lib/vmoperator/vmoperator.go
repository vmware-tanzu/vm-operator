// Copyright (c) 2019-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net"
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	netopv1alpha1 "github.com/vmware-tanzu/vm-operator/external/net-operator/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/vm-operator/external/nsx-operator/api/vpc/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/test/e2e/framework"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
)

const virtualMachineKind = "VirtualMachine"

const (
	vmopConfigurationConfigMap  = "vsphere.provider.config.vmoperator.vmware.com"
	vmClassBestEffortSmall      = "best-effort-small"
	vmClassBestEffortExtraSmall = "best-effort-xsmall"
	NetworkProviderTypeVPC      = "NSXT_VPC"
)

// NetworkProviderInfo contains the network provider information for a VM.
type NetworkProviderInfo struct {
	NetworkType string
	IPv4        string
	SubnetMask  string
	Gateway     string
}

func IsNetworkNsxtVPC(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig) bool {
	envs, err := utils.GetCommandEnvVars(ctx, client, config.GetVariable("VMOPNamespace"), config.GetVariable("VMOPDeploymentName"), config.GetVariable("VMOPManagerCommand"))
	Expect(err).ToNot(HaveOccurred(), "%q cannot not be fetched from %q", config.GetVariable("EnvNetworkProvider"), config.GetVariable("VMOPManagerCommand"))

	return envs[config.GetVariable("EnvNetworkProvider")] == NetworkProviderTypeVPC
}

// Utility function to ensure that a VirtualMachine with given name either exists or not, returns as soon as the CR exists in etcd.
// To check if the vSphere VM has been created, see WaitForVirtualMachineConditionCreated.
func WaitForVirtualMachineToExist(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) {
	By("Verifying the existence of VM CR in etcd")
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return vm != nil
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(BeTrue(), "Timed out waiting for k8s VirtualMachine %s to exist", vmName)
}

// Utility function to wait for a VM to exist, VirtualMachineCreated condition to exist and expect to be True.
// This function fails when VirtualMachineCreated.Status == ConditionFalse, meaning the vSphere VM failed to be created.
// Use this function before helpers that wait on vSphere VM properties,
// such as WaitForVirtualMachinePowerState and WaitForVirtualMachineIP.
func WaitForVirtualMachineConditionCreated(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) {
	kind := vmopv1a3.VirtualMachineConditionCreated

	By("Waiting for vSphere VM to be created")
	Eventually(func(g Gomega) bool {
		vm, err := utils.GetVirtualMachineA3(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		actualCondition := GetVirtualMachineConditionA3(vm, kind)
		g.Expect(actualCondition).ToNot(BeNil())
		g.Expect(actualCondition.Status).To(Equal(metav1.ConditionTrue))

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(BeTrue(), "Timed out waiting for VirtualMachine %s to be created", vmName)
}

// Utility function to wait for the VM's Status.Class.Name to be updated.
func WaitForVirtualMachineStatusClassUpdated(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName, className string) {
	Eventually(func(g Gomega) bool {
		vm, err := utils.GetVirtualMachineA3(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		g.Expect(vm.Status.Class).NotTo(BeNil())
		g.Expect(vm.Status.Class.Name).To(Equal(className))

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(BeTrue(), "Timed out waiting for VirtualMachines %s Status.Class to be updated to %s", vmName, className)
}

func UpdateVirtualMachineClassName(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName, className string) {
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		vm.Spec.ClassName = className
		if err := client.Update(ctx, vm); err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-resize")...).Should(BeTrue(), "Timed out updating VirtualMachines %s ClassName to %s", vmName, className)
}

func GetVirtualMachineCondition(vm *vmopv1a2.VirtualMachine, conditionType string) *metav1.Condition {
	for _, condition := range vm.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}

	return nil
}

// Utility function to check a particular condition consistency on a given list of Virtual Machine.
func CheckVirtualMachinesConditionConsistent(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client,
	ns string, vmName string, expectedCondition metav1.Condition) {
	Consistently(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		g.Expect(err).ToNot(HaveOccurred())

		actualCondition := GetVirtualMachineCondition(vm, expectedCondition.Type)
		g.Expect(actualCondition).ToNot(BeNil())
		g.Expect(actualCondition.Status).Should(Equal(expectedCondition.Status))

		if actualCondition.Status == metav1.ConditionFalse {
			g.Expect(actualCondition.Reason).Should(Equal(expectedCondition.Reason))
		}
	}, config.GetIntervals("default", "consistent-virtual-machine-condition")...).Should(Succeed(), "VirtualMachine conditions changed")
}

func GetVirtualMachineConditionA3(vm *vmopv1a3.VirtualMachine, conditionType string) *metav1.Condition {
	var condition *metav1.Condition

	for _, c := range vm.Status.Conditions {
		if c.Type == conditionType {
			if condition != nil {
				if condition.LastTransitionTime.After(c.LastTransitionTime.Time) {
					continue
				}
			}

			condition = &c
		}
	}

	return condition
}

// Utility function to check Virtual Machine creation.
func WaitForVirtualMachineCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmName string) {
	By(fmt.Sprintf("Verify that a single VirtualMachine '%s/%s' is created", ns, vmName))
	WaitForVirtualMachineToExist(ctx, config, svClusterClient, ns, vmName)
	WaitForVirtualMachineConditionCreated(ctx, config, svClusterClient, ns, vmName)
	WaitForVirtualMachinePowerState(ctx, config, svClusterClient, ns, vmName, string(vmopv1a5.VirtualMachinePowerStateOn))
	WaitForVirtualMachineIP(ctx, config, svClusterClient, ns, vmName)
}

// Utility function to check Virtual Machine Status IP.
func WaitForVirtualMachineIP(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmName string) {
	By(fmt.Sprintf("Verify that an IP (ipv4) is allocated to the VirtualMachine '%s/%s'", ns, vmName))
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return vm.Status.Network != nil &&
			vm.Status.Network.PrimaryIP4 != "" &&
			net.ParseIP(vm.Status.Network.PrimaryIP4).To4() != nil
	}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(BeTrue())
}

// Utility function to check Virtual Machine Status MoID.
func WaitForVirtualMachineMOID(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmName string) {
	By("Verify that the VirtualMachine has a Unique ID/MOID")
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return vm.Status.UniqueID != ""
	}, config.GetIntervals("default", "wait-virtual-machine-moid")...).Should(BeTrue())
}

// Utility function to check PVC Attachment with Virtual Machine Status.
func WaitForPVCAttachment(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmName, pvcName string) {
	By("Verify that the VirtualMachine has a PVC attachment: " + pvcName)
	// Note that we rely on the assumption that the volume name is the
	// same as the PVC name. This is enforced by the VM manifest
	// builder that uses the PVC name as the volume name.
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		for _, vol := range vm.Status.Volumes {
			if strings.HasPrefix(vol.Name, pvcName) {
				return vol.Attached
			}
		}

		return false
	}, config.GetIntervals("default", "wait-pvc-attachment")...).Should(BeTrue())
}

// Utility function to check Virtual Machine Instance Storage Annotations.
func WaitForVirtualMachineInstanceStorageAnnotations(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmName string) {
	By("Verify that we have a single VirtualMachine")
	WaitForVirtualMachineToExist(ctx, config, svClusterClient, ns, vmName)

	By("Verify that VirtualMachine is up and running in vSphere and Instance Storage Annotations are added")
	Eventually(func(g Gomega) bool {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		g.Expect(vm.GetAnnotations()).To(HaveKey("vmoperator.vmware.com/instance-storage-selected-node"))

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(BeTrue())
}

// Utility function to ensure that a condition on a VirtualMachine resource eventually reaches the expected status.
func WaitOnVirtualMachineConditionUpdate(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string,
	expectedCondition metav1.Condition) {
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		actualCondition := GetVirtualMachineCondition(vm, expectedCondition.Type)
		if actualCondition == nil {
			return false
		}

		if actualCondition.Status != expectedCondition.Status {
			// wait and retry condition fetch in a while
			return false
		}

		if actualCondition.Status == metav1.ConditionFalse {
			// Wait for reason eventually to become the expected one.
			// When we delete a VMClass, if WCP_Namespaced_VM_Class is not enabled,
			// the reason may be VirtualMachineClassBindingNotFound at first,
			// but it would eventually be VirtualMachineClassNotFound
			if actualCondition.Reason != expectedCondition.Reason {
				return false
			}
		}

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(BeTrue(), "Timed out waiting for VirtualMachines %s condition to be updated", vmName)
}

// Utility function to check a particular condition on Virtual Machine creation.
func WaitOnVirtualMachineCondition(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, vmName string,
	expectedCondition metav1.Condition) {
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		g.Expect(err).ToNot(HaveOccurred())

		actualCondition := GetVirtualMachineCondition(vm, expectedCondition.Type)
		g.Expect(actualCondition).ToNot(BeNil())

		g.Expect(actualCondition.Status).Should(Equal(expectedCondition.Status))

		if actualCondition.Status == metav1.ConditionFalse {
			g.Expect(actualCondition.Reason).Should(Equal(expectedCondition.Reason))
		}
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(Succeed(), "Timed out waiting for Condition: %+v on VirtualMachine: %s", expectedCondition, vmName)
}

// Utility function to ensure that a VirtualMachine is deleted.
func WaitForVirtualMachineToBeDeleted(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) {
	Eventually(func(g Gomega) {
		_, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}, config.GetIntervals("default", "wait-virtual-machine-deletion")...).Should(Succeed(), "Timed out waiting for VirtualMachine %s to be deleted", vmName)
}

// Utility function to ensure that a Subnet/SubnetSet is deleted.
func WaitForSubnetOrSubnetSetToBeDeleted(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, subnetName, kind string) {
	switch kind {
	case "Subnet":
		Eventually(func(g Gomega) {
			_, err := utils.GetSubnet(ctx, client, ns, subnetName)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, config.GetIntervals("default", "wait-subnet-deletion")...).Should(Succeed(), "Timed out waiting for Subnet %s to be deleted", subnetName)
	case "SubnetSet":
		Eventually(func(g Gomega) {
			_, err := utils.GetSubnetSet(ctx, client, ns, subnetName)
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, config.GetIntervals("default", "wait-subnet-deletion")...).Should(Succeed(), "Timed out waiting for SubnetSet %s to be deleted", subnetName)
	default:
		Expect(false).To(BeTrue(), "unknown kind: %s", kind)
	}
}

func WaitForVirtualMachineZone(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, zone, vmName string) {
	Eventually(func(g Gomega) bool {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		if zone == "" {
			g.Expect(vm.Status.Zone).ToNot(BeEmpty())
		} else {
			g.Expect(vm.Status.Zone).To(Equal(zone))
		}

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-creation")...).Should(BeTrue(), "Timed out waiting for VirtualMachines %s to be created in zone %s", vmName, zone)
}

func UpdateVirtualMachinePowerState(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName, powerState string) {
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		vm.Spec.PowerState = vmopv1a2.VirtualMachinePowerState(powerState)
		if err := client.Update(ctx, vm); err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return true
	}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(BeTrue(), "Timed out updating VirtualMachines %s PowerState to %s", vmName, powerState)
}

func WaitForVirtualMachinePowerState(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, vmName, expectedPowerState string) {
	By(fmt.Sprintf("Waiting for VM power state to reach %s", expectedPowerState))
	Eventually(func() bool {
		vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
		if err != nil {
			e2eframework.Logf("retry due to: %v", err)
			return false
		}

		return vm.Status.PowerState == vmopv1a2.VirtualMachinePowerState(expectedPowerState)
	}, config.GetIntervals("default", "wait-virtual-machine-powerstate")...).Should(BeTrue(), "Timed out waiting for VirtualMachines %s PowerState to be updated to %s", vmName, expectedPowerState)
}

func WaitForLinuxPrepCustomizeNextPowerOnFalse(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, vmName string) {
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA5(ctx, client, ns, vmName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Spec.Bootstrap).ToNot(BeNil())
		g.Expect(vm.Spec.Bootstrap.LinuxPrep).ToNot(BeNil())
		g.Expect(vm.Spec.Bootstrap.LinuxPrep.CustomizeAtNextPowerOn).To(HaveValue(BeFalse()))
	}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(Succeed(), "Timed out waiting for VirtualMachines %s LinuxPrep CustomizeNextPowerOn to be updated to be false", vmName)
}

func GetVirtualMachineMOID(ctx context.Context, client ctrlclient.Client, ns, vmName string) string {
	vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(vm.Status.UniqueID).ShouldNot(BeEmpty())

	return vm.Status.UniqueID
}

func GetVirtualMachineIP(ctx context.Context, client ctrlclient.Client, ns, vmName string) string {
	vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
	Expect(err).ShouldNot(HaveOccurred())

	if vm.Status.Network == nil {
		return ""
	}

	return vm.Status.Network.PrimaryIP4
}

func DeleteVirtualMachine(ctx context.Context, client ctrlclient.Client, ns, vmName string) {
	vm, err := utils.GetVirtualMachine(ctx, client, ns, vmName)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(client.Delete(ctx, vm)).Should(Succeed())
}

func DeleteSubnetOrSubnetSet(ctx context.Context, client ctrlclient.Client, ns, subnetName, kind string) {
	var (
		obj ctrlclient.Object
		err error
	)

	switch kind {
	case "Subnet":
		obj, err = utils.GetSubnet(ctx, client, ns, subnetName)
	case "SubnetSet":
		obj, err = utils.GetSubnetSet(ctx, client, ns, subnetName)
	default:
		err = fmt.Errorf("unsupported kind: %s", kind)
	}

	Expect(err).ToNot(HaveOccurred())
	Expect(client.Delete(ctx, obj)).To(Succeed())
}

// DescribeAllVirtualMachinesInNamespace logs the output of `kubectl describe vm` for all VMs in the given namespace.
func DescribeAllVirtualMachinesInNamespace(ctx context.Context, client ctrlclient.Client, kubeconfigPath, ns string) {
	e2eframework.Logf("Describing all VMs in namespace %s", ns)

	vmList := &vmopv1a2.VirtualMachineList{}
	Expect(client.List(ctx, vmList, ctrlclient.InNamespace(ns))).Should(Succeed())

	for _, vm := range vmList.Items {
		stdout, stderr, err := framework.KubectlDescribeWithNamespacedName(ctx, kubeconfigPath, "vm", ns, vm.Name)
		if err != nil {
			e2eframework.Logf("Failed to run kubectl describe for VM '%s/%s': %s", ns, vm.Name, stderr)
			continue
		}

		e2eframework.Logf("kubectl describe vm -n %s %s:\n%s", ns, vm.Name, stdout)
	}
}

// DescribeResourceIfExists logs the output of `kubectl describe <resource>` if the given resource exists.
func DescribeResourceIfExists(ctx context.Context, client ctrlclient.Client, kubeconfigPath, ns, resourceName, resource string) {
	stdout, stderr, err := framework.KubectlDescribeWithNamespacedName(ctx, kubeconfigPath, resource, ns, resourceName)
	if bytes.Contains(stderr, []byte("NotFound")) {
		e2eframework.Logf("Skip kubectl describe output as the resource %s '%s/%s' doesn't exist", resource, ns, resourceName)
		return
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to run kubectl describe for resource %s '%s/%s': %s", resource, ns, resourceName, stderr)
	e2eframework.Logf("kubectl describe %s -n %s %s:\n%s", resource, ns, resourceName, stdout)
}

// Utility function to get a image k8s name given its display name.
func WaitForVirtualMachineImageName(ctx context.Context, config *framework.Config,
	client ctrlclient.Client, namespace, imageDisplayName string) (string, error) {
	vmImageRegistryFss := utils.IsFssEnabled(ctx, client,
		config.GetVariable("VMOPNamespace"),
		config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"),
		config.GetVariable("EnvFSSVMImageRegistry"))

	var options []ctrlclient.ListOption
	if vmImageRegistryFss {
		options = append(options, ctrlclient.InNamespace(namespace))
	}

	var imageName string

	Eventually(func(g Gomega) bool {
		imgList, err := utils.ListVirtualMachineImagesWithOptions(ctx, client, options)
		g.Expect(err).ToNot(HaveOccurred(), "Failed to list VirtualMachineImages")

		for _, img := range imgList.Items {
			if img.Status.Name == imageDisplayName {
				imageName = img.Name
				return true
			}
		}

		return false
	}, config.GetIntervals("default", "wait-virtual-machine-image-creation")...).Should(BeTrue(),
		fmt.Sprintf("failed to find vm image with display name %s", imageDisplayName))

	return imageName, nil
}

// Utility function to wait for a VMI to have its Status.Disks populated.
func WaitForVirtualMachineImageStatusDisks(ctx context.Context, config *framework.Config,
	client ctrlclient.Client, namespace, imageName string) {
	vmImageRegistryFss := utils.IsFssEnabled(ctx, client,
		config.GetVariable("VMOPNamespace"),
		config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"),
		config.GetVariable("EnvFSSVMImageRegistry"))
	if !vmImageRegistryFss {
		namespace = ""
	}

	objKey := ctrlclient.ObjectKey{Name: imageName, Namespace: namespace}

	Eventually(func(g Gomega) {
		vmi := &vmopv1a3.VirtualMachineImage{}
		g.Expect(client.Get(ctx, objKey, vmi)).To(Succeed())
		g.Expect(vmi.Status.Disks).ToNot(BeEmpty())
	}, config.GetIntervals("default", "wait-virtual-machine-image-creation")...).Should(Succeed(),
		fmt.Sprintf("failed to wait for vm image %s to have Status.Disks populated", imageName))
}

// Utility function to get a ClusterVirtualMachineImage k8s object's name by its display name.
func WaitForClusterVirtualMachineImageName(ctx context.Context, config *framework.Config,
	client ctrlclient.Client, imageDisplayName string) (string, error) {
	vmImageRegistryFss := utils.IsFssEnabled(ctx, client,
		config.GetVariable("VMOPNamespace"),
		config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"),
		config.GetVariable("EnvFSSVMImageRegistry"))
	if !vmImageRegistryFss {
		return "", fmt.Errorf("cannot get ClusterVirtualMachineImage as image-registry FSS is not enabled")
	}

	var cvmiName string

	Eventually(func(g Gomega) bool {
		cvmiList, err := utils.ListClusterVirtualMachineImages(ctx, client)
		g.Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterVirtualMachineImages")

		for _, cvmiObj := range cvmiList.Items {
			if cvmiObj.Status.Name == imageDisplayName {
				cvmiName = cvmiObj.Name
				return true
			}
		}

		return false
	}, config.GetIntervals("default", "wait-virtual-machine-image-creation")...).Should(BeTrue(),
		fmt.Sprintf("failed to find cvmi by display name %s", imageDisplayName))

	return cvmiName, nil
}

func GetClusterScopedVirtualMachineImage(ctx context.Context, config *framework.Config, client ctrlclient.Client, name string) (ctrlclient.Object, error) {
	vmImageRegistryFss := utils.IsFssEnabled(ctx, client,
		config.GetVariable("VMOPNamespace"),
		config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"),
		config.GetVariable("EnvFSSVMImageRegistry"))
	if vmImageRegistryFss {
		virtualMachineImage := &vmopv1a2.ClusterVirtualMachineImage{}

		err := client.Get(ctx, ctrlclient.ObjectKey{Name: name}, virtualMachineImage)
		if err != nil {
			return nil, err
		}

		return virtualMachineImage, nil
	}

	virtualMachineImage := &vmopv1a2.VirtualMachineImage{}

	err := client.Get(ctx, ctrlclient.ObjectKey{Name: name}, virtualMachineImage)
	if err != nil {
		return nil, err
	}

	return virtualMachineImage, nil
}

func GetClusterScopedVirtualMachineImageV1A1(ctx context.Context, config *framework.Config, client ctrlclient.Client, name string) (ctrlclient.Object, error) {
	vmImageRegistryFss := utils.IsFssEnabled(ctx, client,
		config.GetVariable("VMOPNamespace"),
		config.GetVariable("VMOPDeploymentName"),
		config.GetVariable("VMOPManagerCommand"),
		config.GetVariable("EnvFSSVMImageRegistry"))
	if vmImageRegistryFss {
		virtualMachineImage := &vmopv1a1.ClusterVirtualMachineImage{}

		err := client.Get(ctx, ctrlclient.ObjectKey{Name: name}, virtualMachineImage)
		if err != nil {
			return nil, err
		}

		return virtualMachineImage, nil
	}

	virtualMachineImage := &vmopv1a1.VirtualMachineImage{}

	err := client.Get(ctx, ctrlclient.ObjectKey{Name: name}, virtualMachineImage)
	if err != nil {
		return nil, err
	}

	return virtualMachineImage, nil
}

func GetVMClassInNamespace(ctx context.Context, client ctrlclient.Client, config *config.E2EConfig, ns, vmclassName string) (*vmopv1a2.VirtualMachineClass, error) {
	vmclass := &vmopv1a2.VirtualMachineClass{}

	e2eframework.Logf("Getting Namespace scoped VMClass %s in namespace %s", vmclassName, ns)

	err := client.Get(ctx, ctrlclient.ObjectKey{Name: vmclassName, Namespace: ns}, vmclass)
	if err != nil {
		return nil, err
	}

	return vmclass, nil
}

func MemoryQuantityToMb(q resource.Quantity) int {
	return int(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

func GetVMClassesNameForUpdate(ctx context.Context, config *framework.Config, client ctrlclient.Client) (oldVMClassName, newVMClassName string, _ error) {
	// By default, we use the name directly from vcenter with the assumption that best-effort-small and
	// best-effort-medium exist, and we cannot assume the vm class CR presents.
	return vmClassBestEffortExtraSmall, vmClassBestEffortSmall, nil
}

func getVirtualMachinePublishRequestCondition(vmPub *vmopv1a2.VirtualMachinePublishRequest, conditionType string) *metav1.Condition {
	for _, condition := range vmPub.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}

	return nil
}

// VerifyVirtualMachinePublishRequestCondition waits until the expected condition is met on the VirtualMachinePublishRequest.
func VerifyVirtualMachinePublishRequestCondition(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, vmPubName string,
	expectedCondition metav1.Condition) {
	Eventually(func(g Gomega) {
		vmPub, err := utils.GetVirtualMachinePublishRequest(ctx, client, ns, vmPubName)
		g.Expect(err).ToNot(HaveOccurred())

		actualCondition := getVirtualMachinePublishRequestCondition(vmPub, expectedCondition.Type)
		g.Expect(actualCondition).ToNot(BeNil())

		g.Expect(actualCondition.Status).Should(Equal(expectedCondition.Status))

		if actualCondition.Status == metav1.ConditionFalse {
			g.Expect(actualCondition.Reason).Should(Equal(expectedCondition.Reason))
		}
	}, config.GetIntervals("default", "wait-virtual-machine-publish-request-condition")...).Should(Succeed(), "Timed out waiting for Condition: %+v on VirtualMachinePublishRequest: %s", expectedCondition, vmPubName)
}

// VerifyVirtualMachineGroupPublishRequestCompleted waits until the completed condition is true.
func VerifyVirtualMachineGroupPublishRequestCompleted(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string) {
	var lastConditions []metav1.Condition

	Eventually(func(g Gomega) bool {
		vmGroupPub, err := utils.GetVirtualMachineGroupPublishRequest(ctx, client, ns, name)
		if err != nil {
			e2eframework.Logf("get vm group publish request error: %v", err)
			return false
		}

		if !reflect.DeepEqual(lastConditions, vmGroupPub.Status.Conditions) {
			lastConditions = vmGroupPub.Status.Conditions
			e2eframework.Logf("VirtualMachineGroupPublishRequest %s Conditions:  %v", name, lastConditions)
		}

		for _, condition := range vmGroupPub.Status.Conditions {
			if condition.Type == vmopv1a5.VirtualMachineGroupPublishRequestConditionComplete {
				return condition.Status == metav1.ConditionTrue
			}
		}

		return false
	}, config.GetIntervals("default", "wait-virtual-machine-group-publish-request-condition")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachineGroupPublishRequest to be completed")
}

func VerifyVirtualMachineGroupPublishRequestDeleted(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string) {
	Eventually(func(g Gomega) bool {
		_, err := utils.GetVirtualMachineGroupPublishRequest(ctx, client, ns, name)
		return apierrors.IsNotFound(err)
	}, config.GetIntervals("default", "wait-virtual-machine-group-publish-request-deletion")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachineGroupPublishRequest to be deleted")
}

func VerifyVirtualMachineGroupLinked(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
	expectedMembers sets.Set[vmopv1a5.GroupMember]) {
	e2eframework.Logf("%s expected members: %v", name, expectedMembers)

	var lastActualMembers sets.Set[vmopv1a5.GroupMember]

	Eventually(func(g Gomega) bool {
		vmGroup, err := utils.GetVirtualMachineGroup(ctx, client, ns, name)
		if err != nil {
			e2eframework.Logf("get vm group error: %v", err)
			return false
		}

		actualMembers := make(sets.Set[vmopv1a5.GroupMember])

		for _, member := range vmGroup.Status.Members {
			for _, condition := range member.Conditions {
				if condition.Type == vmopv1a5.VirtualMachineGroupMemberConditionGroupLinked &&
					condition.Status == metav1.ConditionTrue {
					actualMembers.Insert(vmopv1a5.GroupMember{
						Name: member.Name,
						Kind: member.Kind,
					})

					break
				}
			}
		}

		if !reflect.DeepEqual(lastActualMembers, actualMembers) {
			lastActualMembers = actualMembers
			e2eframework.Logf("%s actual members: %v", name, lastActualMembers)
		}

		return actualMembers.Equal(expectedMembers)
	}, config.GetIntervals("default", "wait-virtual-machine-group-condition-update")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachineGroup to have expected members with group linked condition")
}

func VerifyWebConsoleRequestStatus(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, webconsoleName string) {
	By("Verify that WebConsoleRequest populates Proxy Address in Status")
	Eventually(func(g Gomega) {
		webconsole, err := utils.GetVWebConsoleRequest(ctx, client, ns, webconsoleName)
		g.Expect(err).ToNot(HaveOccurred())

		actualStatusProxyAddr := webconsole.Status.ProxyAddr
		g.Expect(actualStatusProxyAddr).ToNot(BeNil())
	}, config.GetIntervals("default", "wait-virtual-machine-web-console-request-creation")...).Should(Succeed(), "Timed out waiting for WebConsoleRequest %s Status to populate proxy addr", webconsoleName)
}

func VerifyVirtualMachineWebConsoleRequestStatus(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmWebconsoleName string) {
	By("Verify that VirtualMachineWebConsoleRequest populates Proxy Address in Status")
	Eventually(func(g Gomega) {
		vmWebconsole, err := utils.GetVirtualMachineWebConsoleRequest(ctx, client, ns, vmWebconsoleName)
		g.Expect(err).ToNot(HaveOccurred())

		actualStatusProxyAddr := vmWebconsole.Status.ProxyAddr
		g.Expect(actualStatusProxyAddr).ToNot(BeNil())
	}, config.GetIntervals("default", "wait-virtual-machine-web-console-request-creation")...).Should(Succeed(), "Timed out waiting for VirtualMachineWebConsoleRequest %s Status to populate proxy addr", vmWebconsoleName)
}

func DeleteVirtualMachinePublishRequest(ctx context.Context, client ctrlclient.Client, ns, vmPubName string) {
	vmPub, err := utils.GetVirtualMachinePublishRequest(ctx, client, ns, vmPubName)
	Expect(err).ToNot(HaveOccurred())
	Expect(client.Delete(ctx, vmPub)).To(Succeed())
}

func WaitForVirtualMachinePublishRequestToBeDeleted(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmPubName string) {
	Eventually(func(g Gomega) {
		_, err := utils.GetVirtualMachinePublishRequest(ctx, client, ns, vmPubName)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}, config.GetIntervals("default", "wait-virtual-machine-publish-request-deletion")...).Should(Succeed(), "Timed out waiting for VirtualMachinePublishRequest %s to be deleted", vmPubName)
}

func GetVirtualMachinePublishRequestSourceName(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmPubName string) (string, error) {
	var vmPubSourceName string

	Eventually(func(g Gomega) {
		vmPub, err := utils.GetVirtualMachinePublishRequest(ctx, svClusterClient, ns, vmPubName)
		g.Expect(err).ToNot(HaveOccurred())

		vmPubStatusSourceRef := vmPub.Status.SourceRef
		g.Expect(vmPubStatusSourceRef).ToNot(BeNil())

		vmPubSourceName = vmPub.Status.SourceRef.Name
		g.Expect(vmPubSourceName).ToNot(BeEmpty())
	}, config.GetIntervals("default", "wait-virtual-machine-publish-request-condition")...).Should(Succeed(), "failed to get vmpub %s source name", vmPubName)

	return vmPubSourceName, nil
}

func GetVirtualMachinePublishRequestTargetItemName(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmPubName string) (string, error) {
	var vmPubTargetItemName string

	Eventually(func(g Gomega) {
		vmPub, err := utils.GetVirtualMachinePublishRequest(ctx, svClusterClient, ns, vmPubName)
		g.Expect(err).ToNot(HaveOccurred())

		vmPubStatusTargetRef := vmPub.Status.TargetRef
		g.Expect(vmPubStatusTargetRef).ToNot(BeNil())

		vmPubTargetItemName = vmPub.Status.TargetRef.Item.Name
		g.Expect(vmPubTargetItemName).ToNot(BeEmpty())
	}, config.GetIntervals("default", "wait-virtual-machine-publish-request-condition")...).Should(Succeed(), "failed to get vmpub %s target item name", vmPubName)

	return vmPubTargetItemName, nil
}

// GetVirtualMachineNetworkProviderIP returns the IP address of the network provider for the given VM.
// For VDS topology, it returns the IP address of the network interface from net-operator.
// For NSX topology, it returns the IP address of the virtual network interface from ncp.
// For VPC topology, it returns the IP address of the subnetport from nsx operator.
func GetVirtualMachineNetworkProviderIP(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) string {
	if framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.VDS) {
		By("Getting VM network provider IP from Net-Operator's networkinterfaces in VDS")

		networkIfList := &netopv1alpha1.NetworkInterfaceList{}
		Expect(client.List(ctx, networkIfList, ctrlclient.InNamespace(ns))).To(Succeed())
		Expect(networkIfList.Items).ToNot(BeEmpty(), "no NetworkInterfaces found in namespace %s", ns)

		for _, networkIf := range networkIfList.Items {
			for _, ownerRef := range networkIf.OwnerReferences {
				if ownerRef.Kind == virtualMachineKind && ownerRef.Name == vmName {
					Expect(networkIf.Status.IPConfigs).ToNot(BeEmpty())
					return networkIf.Status.IPConfigs[0].IP
				}
			}
		}
	}

	if framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.NSX) {
		if IsNetworkNsxtVPC(ctx, client, config) {
			By("Getting VM network provider IP from NSX Operator's SubnetPort in NSX")

			subnetPortList := &vpcv1alpha1.SubnetPortList{}
			Expect(client.List(ctx, subnetPortList, ctrlclient.InNamespace(ns))).To(Succeed())
			Expect(subnetPortList.Items).ToNot(BeEmpty(), "no SubnetPort found in namespace %s", ns)

			for _, subnetPort := range subnetPortList.Items {
				for _, ownerRef := range subnetPort.OwnerReferences {
					if ownerRef.Kind == virtualMachineKind && ownerRef.Name == vmName {
						Expect(subnetPort.Status.NetworkInterfaceConfig.IPAddresses).ToNot(BeEmpty())
						// Note SubnetPort provides IPAddress with CIDR format.
						cidr := subnetPort.Status.NetworkInterfaceConfig.IPAddresses[0].IPAddress
						ip, _, err := net.ParseCIDR(cidr)
						Expect(err).ToNot(HaveOccurred(), "failed to parse CIDR from SubnetPort IPAddress %s", cidr)

						return ip.String()
					}
				}
			}
		} else {
			By("Getting VM network provider IP from NCP's VirtualNetworkInterfaces in NSX")

			vnetIfList := &ncpv1alpha1.VirtualNetworkInterfaceList{}
			Expect(client.List(ctx, vnetIfList, ctrlclient.InNamespace(ns))).To(Succeed())
			Expect(vnetIfList.Items).ToNot(BeEmpty(), "no VirtualNetworkInterfaces found in namespace %s", ns)

			for _, vnetIf := range vnetIfList.Items {
				for _, ownerRef := range vnetIf.OwnerReferences {
					if ownerRef.Kind == virtualMachineKind && ownerRef.Name == vmName {
						Expect(vnetIf.Status.IPAddresses).ToNot(BeEmpty())
						return vnetIf.Status.IPAddresses[0].IP
					}
				}
			}
		}
	}

	return ""
}

func waitForVDSNetworkIf(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) []NetworkProviderInfo {
	var res []NetworkProviderInfo

	Eventually(func() error {
		networkIfList := &netopv1alpha1.NetworkInterfaceList{}

		err := client.List(ctx, networkIfList, ctrlclient.InNamespace(ns))
		if err != nil {
			return err
		}

		if len(networkIfList.Items) == 0 {
			return fmt.Errorf("no NetworkInterfaces found in namespace %s", ns)
		}

		for _, networkIf := range networkIfList.Items {
			for _, ownerRef := range networkIf.OwnerReferences {
				if ownerRef.Kind == virtualMachineKind && ownerRef.Name == vmName {
					if len(networkIf.Status.IPConfigs) == 0 {
						return fmt.Errorf("no IPConfigs found for NetworkInterface %s", networkIf.Name)
					}

					res = make([]NetworkProviderInfo, len(networkIf.Status.IPConfigs))
					for i, ipConfig := range networkIf.Status.IPConfigs {
						res[i] = NetworkProviderInfo{
							NetworkType: consts.VDSNetworkType,
							IPv4:        ipConfig.IP,
							SubnetMask:  ipConfig.SubnetMask,
							Gateway:     ipConfig.Gateway,
						}
					}

					return nil
				}
			}
		}

		return fmt.Errorf("no NetworkInterface found for VirtualMachine %s", vmName)
	}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(Succeed())

	return res
}

func waitForNSXVirtualNetworkIf(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) []NetworkProviderInfo {
	var res []NetworkProviderInfo

	Eventually(func() error {
		vnetIfList := &ncpv1alpha1.VirtualNetworkInterfaceList{}

		err := client.List(ctx, vnetIfList, ctrlclient.InNamespace(ns))
		if err != nil {
			return err
		}

		if len(vnetIfList.Items) == 0 {
			return fmt.Errorf("no VirtualNetworkInterfaces found in namespace %s", ns)
		}

		for _, vnetIf := range vnetIfList.Items {
			for _, ownerRef := range vnetIf.OwnerReferences {
				if ownerRef.Kind == virtualMachineKind && ownerRef.Name == vmName {
					if len(vnetIf.Status.IPAddresses) == 0 {
						return fmt.Errorf("no IPAddresses found for VirtualNetworkInterface %s", vnetIf.Name)
					}

					res = make([]NetworkProviderInfo, len(vnetIf.Status.IPAddresses))
					for i, ipAddr := range vnetIf.Status.IPAddresses {
						res[i] = NetworkProviderInfo{
							NetworkType: consts.NSXNetworkType,
							IPv4:        ipAddr.IP,
							SubnetMask:  ipAddr.SubnetMask,
							Gateway:     ipAddr.Gateway,
						}
					}

					return nil
				}
			}
		}

		return fmt.Errorf("no VirtualNetworkInterface found for VirtualMachine %s", vmName)
	}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(Succeed())

	return res
}

func waitForSubnetPort(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) []NetworkProviderInfo {
	var res []NetworkProviderInfo

	Eventually(func() error {
		subnetPortList := &vpcv1alpha1.SubnetPortList{}

		err := client.List(ctx, subnetPortList, ctrlclient.InNamespace(ns))
		if err != nil {
			return err
		}

		if len(subnetPortList.Items) == 0 {
			return fmt.Errorf("no SubnetPort found in namespace %s", ns)
		}

		for _, subnetPort := range subnetPortList.Items {
			for _, ownerRef := range subnetPort.OwnerReferences {
				if ownerRef.Kind == virtualMachineKind && ownerRef.Name == vmName {
					if len(subnetPort.Status.NetworkInterfaceConfig.IPAddresses) == 0 {
						return fmt.Errorf("no IPAddresses found for SubnetPort %s", subnetPort.Name)
					}

					res = make([]NetworkProviderInfo, len(subnetPort.Status.NetworkInterfaceConfig.IPAddresses))
					for i, ipAddr := range subnetPort.Status.NetworkInterfaceConfig.IPAddresses {
						ip, ipNet, err := net.ParseCIDR(ipAddr.IPAddress)
						if err != nil || ipNet == nil {
							return fmt.Errorf("failed to parse CIDR from SubnetPort IPAddress %s", ipAddr.IPAddress)
						}

						res[i] = NetworkProviderInfo{
							NetworkType: consts.VPCNetworkType,
							IPv4:        ip.String(),
							SubnetMask:  net.IP(ipNet.Mask).String(),
							Gateway:     ipAddr.Gateway,
						}
					}

					return nil
				}
			}
		}

		return fmt.Errorf("no SubnetPort found for VirtualMachine %s", vmName)
	}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(Succeed())

	return res
}

// WaitForVMNetworkProviderInfo waits for the network provider (NetworkInterface in VDS, VirtualNetworkInterface in NSX, Subnetport in VPC)
// to be created and contain the network information for the given VM.
func WaitForVMNetworkProviderInfo(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, vmName string) []NetworkProviderInfo {
	var res []NetworkProviderInfo

	if framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.VDS) {
		By("Getting VM network provider IP from Net-Operator's networkinterfaces in VDS")

		res = waitForVDSNetworkIf(ctx, config, client, ns, vmName)
	} else if framework.NetworkTopologyIs(config.InfraConfig.NetworkingTopology, consts.NSX) {
		if IsNetworkNsxtVPC(ctx, client, config) {
			By("Getting VM network provider IP from NSX Operator's SubnetPort in NSX")

			res = waitForSubnetPort(ctx, config, client, ns, vmName)
		} else {
			By("Getting VM network provider IP from NCP's VirtualNetworkInterfaces in NSX")

			res = waitForNSXVirtualNetworkIf(ctx, config, client, ns, vmName)
		}
	}

	return res
}

func WaitOnVirtualMachineGroupCondition(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, groupName string,
	expectedCondition metav1.Condition) {
	Eventually(func(g Gomega) bool {
		vmg, err := utils.GetVirtualMachineGroup(ctx, client, ns, groupName)
		g.Expect(err).ToNot(HaveOccurred())

		for _, c := range vmg.Status.Conditions {
			if c.Type == expectedCondition.Type {
				g.Expect(c.Status).Should(Equal(expectedCondition.Status))

				if expectedCondition.Reason != "" {
					g.Expect(c.Reason).Should(Equal(expectedCondition.Reason))
				}

				return true
			}
		}

		return false
	}, config.GetIntervals("default", "wait-virtual-machine-group-condition-update")...).Should(BeTrue(), "Timed out waiting for Condition: %+v on VirtualMachineGroup: %q", expectedCondition, groupName)
}

func WaitOnVirtualMachineGroupMemberCondition(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, groupName, memberName, memberKind string,
	expectedCondition metav1.Condition) {
	Eventually(func(g Gomega) error {
		vmg, err := utils.GetVirtualMachineGroup(ctx, client, ns, groupName)
		g.Expect(err).ToNot(HaveOccurred())

		for _, m := range vmg.Status.Members {
			if m.Name == memberName && m.Kind == memberKind {
				for _, c := range m.Conditions {
					if c.Type == expectedCondition.Type {
						g.Expect(c.Status).Should(Equal(expectedCondition.Status))

						if expectedCondition.Reason != "" {
							g.Expect(c.Reason).Should(Equal(expectedCondition.Reason))
						}

						return nil
					}
				}

				return fmt.Errorf("condition type %s not found for member %s/%s in VirtualMachineGroup %s", expectedCondition.Type, memberKind, memberName, groupName)
			}
		}

		return fmt.Errorf("member %s/%s not found in VirtualMachineGroup %s", memberKind, memberName, groupName)
	}, config.GetIntervals("default", "wait-virtual-machine-group-condition-update")...).Should(Succeed(), "Timed out waiting for member %s/%s condition: %+v on VirtualMachineGroup: %s", memberKind, memberName, expectedCondition, groupName)
}

func WaitForVirtualMachineGroupToBeDeleted(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, groupName string) {
	Eventually(func(g Gomega) {
		_, err := utils.GetVirtualMachineGroup(ctx, client, ns, groupName)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	}, config.GetIntervals("default", "wait-virtual-machine-group-deletion")...).Should(Succeed(), "Timed out waiting for VirtualMachineGroup %s to be deleted", groupName)
}

func VerifyVirtualMachineSnapshotDeleted(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string) {
	GinkgoHelper()

	Eventually(func(g Gomega) bool {
		_, err := utils.GetVirtualMachineSnapshot(ctx, client, ns, name)
		return apierrors.IsNotFound(err)
	}, config.GetIntervals("default", "wait-virtual-machine-snapshot-deletion")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachineSnapshot to be deleted")
}

func VerifyVirtualMachineSnapshotCondition(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
	powerState vmopv1a5.VirtualMachinePowerState,
	quiesced bool,
	children []vmopv1a5.VirtualMachineSnapshotReference,
) {
	GinkgoHelper()

	var lastConditions []metav1.Condition
	Eventually(func(g Gomega) bool {
		vmSnapshot, err := utils.GetVirtualMachineSnapshot(ctx, client, ns, name)
		if err != nil {
			e2eframework.Logf("get vm snapshot error: %v", err)
			return false
		}

		if !reflect.DeepEqual(lastConditions, vmSnapshot.Status.Conditions) {
			e2eframework.Logf("VirtualMachineSnapshot %s Conditions changed: %v", name, vmSnapshot.Status.Conditions)
			lastConditions = vmSnapshot.Status.Conditions
		}

		for _, condition := range vmSnapshot.Status.Conditions {
			if condition.Type == vmopv1a5.VirtualMachineSnapshotReadyCondition {
				return condition.Status == metav1.ConditionTrue
			}
		}

		return false
	}, config.GetIntervals("default", "wait-virtual-machine-snapshot-condition")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachineSnapshot to be ready, current conditions: %v", lastConditions)

	Eventually(func(g Gomega) {
		vmSnapshot, err := utils.GetVirtualMachineSnapshot(ctx, client, ns, name)
		g.Expect(err).To(Succeed())
		g.Expect(vmSnapshot.Status.UniqueID).NotTo(BeEmpty())
		g.Expect(vmSnapshot.Status.PowerState).To(Equal(powerState),
			"PowerState is not equal to %v", powerState)
		g.Expect(vmSnapshot.Status.Quiesced).To(Equal(quiesced),
			"Quiesced is not equal to %v", quiesced)
		g.Expect(vmSnapshot.Status.Children).To(HaveLen(len(children)),
			"Children is not equal to %v", children)
		g.Expect(vmSnapshot.Status.Children).To(ConsistOf(children),
			"Children is not equal to %v", children)
		g.Expect(vmSnapshot.Status.Storage).NotTo(BeNil(), "Storage is nil")
		g.Expect(vmSnapshot.Status.Storage.Used).NotTo(BeNil(), "Used is nil")
		g.Expect(vmSnapshot.Status.Storage.Requested).NotTo(BeNil(), "Requested is nil")
	}, config.GetIntervals("default", "wait-virtual-machine-snapshot-condition")...).Should(Succeed(),
		"Timed out waiting for VirtualMachineSnapshot's condition")
}

func VerifySnapshotStatusOnVirtualMachine(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
	currentSnapshot *vmopv1a5.VirtualMachineSnapshotReference,
	rootSnapshots []vmopv1a5.VirtualMachineSnapshotReference,
	powerState vmopv1a5.VirtualMachinePowerState,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA5(ctx, client, ns, name)
		g.Expect(err).To(Succeed())
		g.Expect(vm.Spec.CurrentSnapshotName).To(BeEmpty(),
			"spec.CurrentSnapshotName should be empty")
		g.Expect(vm.Status.CurrentSnapshot).To(Equal(currentSnapshot),
			"CurrentSnapshot is not equal to %v", currentSnapshot)
		g.Expect(vm.Status.RootSnapshots).To(HaveLen(len(rootSnapshots)),
			"RootSnapshots's length is not equal to %v", len(rootSnapshots))
		g.Expect(vm.Status.RootSnapshots).To(ConsistOf(rootSnapshots),
			"RootSnapshots is not equal to %v", rootSnapshots)
		g.Expect(vm.Status.PowerState).To(Equal(powerState),
			"PowerState is not equal to %v", powerState)
	}, config.GetIntervals("default", "wait-virtual-machine-snapshot-related-resource")...).Should(Succeed(),
		"Timed out waiting for VirtualMachineSnapshot's related resource")
}

func VerifyVirtualMachineRestartMode(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	ns, name string,
	restartMode vmopv1a5.VirtualMachinePowerOpMode,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA5(ctx, client, ns, name)
		g.Expect(err).To(Succeed())
		g.Expect(vm.Spec.RestartMode).To(Equal(restartMode))
	}, config.GetIntervals("default", "wait-virtual-machine-snapshot-related-resource")...).Should(Succeed(),
		"Timed out waiting for restart mode of VM")
}

func VerifyVMSnapshotDeletion(
	ctx context.Context,
	client ctrlclient.Client,
	vmSvcE2EConfig *config.E2EConfig,
	params manifestbuilders.VirtualMachineSnapshotYaml,
) {
	GinkgoHelper()

	Eventually(func() bool {
		_, err := utils.GetVirtualMachineSnapshot(ctx, client, params.Namespace, params.Name)
		return apierrors.IsNotFound(err)
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-snapshot-deletion")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachineSnapshot to be deleted")
}

func VerifyVMSnapshotQuotaUsage(
	ctx context.Context,
	client ctrlclient.Client,
	vmSvcE2EConfig *config.E2EConfig,
	ns string,
	spuName string,
	snapshots ...string,
) {
	GinkgoHelper()

	Eventually(func(g Gomega) {
		usageTotal := resource.NewQuantity(0, resource.BinarySI)

		for _, snapshot := range snapshots {
			snapshotCR, err := utils.GetVirtualMachineSnapshot(ctx, client, ns, snapshot)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(snapshotCR.Status).NotTo(BeNil())
			g.Expect(snapshotCR.Status.Storage).NotTo(BeNil())
			g.Expect(snapshotCR.Status.Storage.Used).NotTo(BeNil())
			usageTotal.Add(*snapshotCR.Status.Storage.Used)
		}

		storagePolicyUsage, err := utils.GetStoragePolicyUsage(ctx, client, ns, spuName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(storagePolicyUsage.Status.ResourceTypeLevelQuotaUsage.Used.Value()).To(Equal(usageTotal.Value()),
			fmt.Sprintf("expect StoragePolicyUsage %s to have correct used capacity ", spuName))
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-snapshot-quota-usage")...).Should(Succeed())
}

func EnsureVMSnapshotDeleted(
	ctx context.Context,
	client ctrlclient.Client,
	vmSvcE2EConfig *config.E2EConfig,
	params manifestbuilders.VirtualMachineSnapshotYaml,
) {
	GinkgoHelper()

	vmSnapshotYaml := manifestbuilders.GetVirtualMachineSnapshotYaml(params)
	e2eframework.Logf("Delete VirtualMachineSnapshot:\n%v", string(vmSnapshotYaml))
	Eventually(func() bool {
		err := utils.DeleteVirtualMachineSnapshot(ctx, client, params.Namespace, params.Name)
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}

		return false
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-snapshot-deletion")...).Should(BeTrue())
}

func VerifyVMDeleted(
	ctx context.Context,
	client ctrlclient.Client,
	vmSvcE2EConfig *config.E2EConfig,
	ns, name string) {
	Eventually(func() bool {
		err := utils.DeleteVirtualMachineA5(ctx, client, ns, name)
		if err != nil && apierrors.IsNotFound(err) {
			return true
		}

		return false
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-deletion")...).Should(BeTrue(),
		"Timed out waiting for VirtualMachine to be deleted")
}
