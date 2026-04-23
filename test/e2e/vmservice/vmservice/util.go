// Copyright (c) 2020-2025 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmservice

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vapi/tags"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/kubernetes/test/e2e/framework"

	e2eframework "github.com/vmware-tanzu/vm-operator/test/e2e/framework"

	capiutil "sigs.k8s.io/cluster-api/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsunregistervolumev1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/cnsunregistervolume/v1alpha1"
	e2essh "github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/ssh"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/testbed"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/e2e/infrastructure/vsphere/wcp"
	"github.com/vmware-tanzu/vm-operator/test/e2e/manifestbuilders"
	"github.com/vmware-tanzu/vm-operator/test/e2e/utils"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/common"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/config"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/consts"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/lib/vmoperator"
)

const (
	trueString = "true"
)

const (
	VMClassNotFound            = "Server error: com.vmware.vapi.std.errors.NotFound"
	VMClassInstanceStorage     = "e2etest-instance-storage-small"
	VMClassE1000               = "e2etest-vmclass-config-e1000"
	VMClassMockVGPUConfigSpec  = "e2etest-vmclass-config-mockvgpu"
	VMClassGPUnDDPIOConfigSpec = "e2etest-vmclass-config-gpuddpio"
	VMClassVMX22               = "e2etest-vmclass-22"
	VMClassReservedVGPU        = "e2etest-vmclass-mockvgpu-reserved"
	VMFinalizerName            = "vmoperator.vmware.com/virtualmachine"
	VMFinalizerNameDeprecated  = "virtualmachine.vmoperator.vmware.com"
)

var (
	vmClassFunctions = map[string]any{
		"e2etest-best-effort-small": CreateSpecE2eTestBestEffortSmall,
		"e2etest-guaranteed-xsmall": CreateSpecE2eTestGuaranteedXSmall,
		"e2etest-guaranteed-medium": CreateSpecE2eTestGuaranteedMedium,
		VMClassE1000:                CreateSpecE2eVMClassE1000,
		VMClassInstanceStorage:      CreateSpecInstanceStorageSmall,
		"best-effort-small":         CreateSpecBestEffortSmall,
		"guaranteed-large":          CreateGuaranteedLarge,
		"customize":                 CreateSpecCustomizedVMClass,
		VMClassMockVGPUConfigSpec:   CreateSpecE2eVMClassMockVGPUConfigSpec,
		VMClassGPUnDDPIOConfigSpec:  CreateSpecE2eVMClassGPUnDDPIOConfigSpec,
		VMClassVMX22:                CreateSpecE2eVMClassVMX22,
		VMClassReservedVGPU:         CreateSpecE2eVMClassReservedVGPU,
	}
)

func EnsureNamespaceHasAccess(wcpClient wcp.WorkloadManagementAPI, vmClassID, ns string) error {
	namespaceInfo, err := wcpClient.GetNamespace(ns)
	if err != nil {
		return err
	}

	if !slices.Contains(namespaceInfo.VMServiceSpec.VMClasses, vmClassID) {
		framework.Logf("VMClass %s is not accessible to namespace %s, adding it", vmClassID, ns)
		namespaceInfo.VMServiceSpec.VMClasses = append(namespaceInfo.VMServiceSpec.VMClasses, vmClassID)

		updateSpec := wcp.NamespaceUpdateVMserviceSpec{VMClasses: &namespaceInfo.VMServiceSpec.VMClasses}

		err := wcpClient.UpdateNamespaceVMServiceSpec(ns, updateSpec)
		if err != nil {
			framework.Logf("Error update namespace %s VMService spec. Err: %s", ns, err)
			return err
		}
	}

	// Wait for namespaceInfo.configStatus to be RUNNING which ensures successful creation of
	// VirtualMachineClass/VirtualMachineClassBinding in the namespace depending on the scope of VirtualMachineClass
	wcp.WaitForNamespaceReady(wcpClient, ns)

	return nil
}

func VerifyVMClassCreate(wcpClient wcp.WorkloadManagementAPI, createSpec wcp.VMClassSpec, expectedSpec wcp.VMClassSpec) {
	Expect(wcpClient.CreateVMClass(createSpec)).To(Succeed())

	var (
		vmClass wcp.VMClassInfo
		err     error
	)
	Eventually(func(g Gomega) {
		vmClass, err = wcpClient.GetVMClassInfo(createSpec.ID)
		g.Expect(err).ToNot(HaveOccurred())
	}, 5*time.Minute, 5*time.Second).Should(Succeed(), "failed to get created vmClass info %w", err)
	VerifyVMClassSpec(vmClass.VMClassSpec, expectedSpec)
}

func VerifyVMClassDeletion(wcpClient wcp.WorkloadManagementAPI, vmClassID string) {
	Expect(wcpClient.DeleteVMClass(vmClassID)).To(Succeed())

	// Eventually VMClass should be deleted.
	Eventually(func(g Gomega) {
		_, err := wcpClient.GetVMClassInfo(vmClassID)
		g.Expect(err).To(HaveOccurred())

		var dcliErr wcp.DcliError
		g.Expect(errors.As(err, &dcliErr)).Should(BeTrue())
		g.Expect(dcliErr.Response()).To(ContainSubstring(VMClassNotFound))
	}, 5*time.Minute, 10*time.Second).Should(Succeed(), "failed to delete vmClass ", vmClassID)
}

func VerifyVMClassSpec(actualVMClassSpec wcp.VMClassSpec, expectedVMClassSpec wcp.VMClassSpec) {
	if expectedVMClassSpec.Description == nil {
		emptyString := ""
		expectedVMClassSpec.Description = &emptyString
	}

	if expectedVMClassSpec.ConfigSpec == nil {
		// wcpsvc sets the ConfigSpec when nil
		expectedVMClassSpec.ConfigSpec = actualVMClassSpec.ConfigSpec
	}

	Expect(actualVMClassSpec).To(BeComparableTo(expectedVMClassSpec), "unexpected class %s", actualVMClassSpec.ID)
}

// GenerateVMClassSpecFunction returns a spec generating function based on vmClassName using map 'vmClassFunctions'.
func GenerateVMClassSpecFunction(vmClassName string, params ...any) (result any, err error) {
	f := reflect.ValueOf(vmClassFunctions[vmClassName])
	if len(params) != f.Type().NumIn() {
		err = errors.New("number of given parameters is out of index")
		return
	}

	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}

	res := f.Call(in)
	result = res[0].Interface()

	return
}

func CreateSpecE2eTestGuaranteedXSmall() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       "e2etest-guaranteed-xsmall",
		CPUCount: new(2),
		MemoryMB: new(512),
	}
}

func CreateSpecE2eTestBestEffortSmall() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       "e2etest-best-effort-small",
		CPUCount: new(2),
		MemoryMB: new(1024),
	}
}

func CreateSpecE2eTestGuaranteedMedium() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       "e2etest-guaranteed-medium",
		CPUCount: new(2),
		MemoryMB: new(8192),
	}
}

func CreateSpecE2eTestGuaranteedXSmallVirtualDevicesVGPUs() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:                "e2etest-guaranteed-xsmall-virtual-devices-vgpus",
		CPUCount:          new(2),
		MemoryMB:          new(512),
		CPUReservation:    new(100),
		MemoryReservation: new(100),
		Devices: wcp.VirtualDevices{
			VGPUDevices: []wcp.VGPUDevice{
				{
					ProfileName: "mockup-vmiop",
				},
				{
					ProfileName: "mockup-vmiop",
				},
			},
		},
	}
}

func CreateSpecE2eTestGuaranteedXSmallCustomizedVirtualDevices(vmClassName string, virtualDevices wcp.VirtualDevices) wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:                vmClassName,
		CPUCount:          new(2),
		MemoryMB:          new(512),
		CPUReservation:    new(100),
		MemoryReservation: new(100),
		Devices:           virtualDevices,
	}
}

// CreateSpecBestEffortSmall creates the best-effort-small VM class spec.
func CreateSpecBestEffortSmall() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       "best-effort-small",
		CPUCount: new(2),
		MemoryMB: new(4096),
	}
}

func CreateGuaranteedLarge() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:                "guaranteed-large",
		CPUCount:          new(4),
		MemoryMB:          new(16 * 1024),
		CPUReservation:    new(100),
		MemoryReservation: new(100),
	}
}

// CreateSpecCustomizedVMClass creates a customized VM class spec with given parameters.
func CreateSpecCustomizedVMClass(className string, cpuCount, memoryMB int, description string) wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:          className,
		CPUCount:    &cpuCount,
		MemoryMB:    &memoryMB,
		Description: &description,
	}
}

// CreateSpecInstanceStorageSmall creates the instance volume VM class spec.
func CreateSpecInstanceStorageSmall(params ...any) wcp.VMClassSpec {
	Expect(params).ToNot(BeEmpty())

	return wcp.VMClassSpec{
		ID:       VMClassInstanceStorage,
		CPUCount: new(2),
		MemoryMB: new(4096),
		InstanceStorage: wcp.InstanceStorage{
			StoragePolicy: params[0].(string),
			Volumes: []wcp.InstanceStorageVolume{
				{
					Size: 2048,
				},
				{
					Size: 1024,
				},
			},
		},
	}
}

// CreateSpecE2eVMClassE1000 creates a vm class spec with a e1000 nic.
func CreateSpecE2eVMClassE1000() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       VMClassE1000,
		CPUCount: new(2),
		MemoryMB: new(4096),
		ConfigSpec: &types.VirtualMachineConfigSpec{
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: types.VirtualDeviceConfigSpecOperationAdd,
					Device: &types.VirtualE1000{
						VirtualEthernetCard: types.VirtualEthernetCard{
							VirtualDevice: types.VirtualDevice{
								Key: -10000,
							},
						},
					},
				},
			},
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{
					Key:   "hello-test-key",
					Value: "hello-test-value",
				},
			},
		},
	}
}

func CreateSpecE2eVMClassVMX22() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       VMClassVMX22,
		CPUCount: new(2),
		MemoryMB: new(4096),
		ConfigSpec: &types.VirtualMachineConfigSpec{
			Version: "vmx-22",
		},
	}
}

func CreateSpecE2eVMClassMockVGPUConfigSpec() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:                VMClassMockVGPUConfigSpec,
		CPUCount:          new(2),
		MemoryMB:          new(512),
		CPUReservation:    new(100),
		MemoryReservation: new(100),
		ConfigSpec: &types.VirtualMachineConfigSpec{
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: types.VirtualDeviceConfigSpecOperationAdd,
					Device: &types.VirtualPCIPassthrough{
						VirtualDevice: types.VirtualDevice{
							Key: -20000,
							Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
								Vgpu: "mockup-vmiop",
							},
						},
					},
				},
			},
		},
	}
}

func CreateSpecE2eVMClassReservedVGPU() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       VMClassReservedVGPU,
		CPUCount: new(1),
		MemoryMB: new(1024),
		ConfigSpec: &types.VirtualMachineConfigSpec{
			NumCPUs:  int32(1),
			MemoryMB: int64(1024),
			CpuAllocation: &types.ResourceAllocationInfo{
				Reservation: new(int64(1000)),
				Limit:       new(int64(1000)),
			},
			MemoryAllocation: &types.ResourceAllocationInfo{
				Reservation: new(int64(1024)),
				Limit:       new(int64(1024)),
			},
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: types.VirtualDeviceConfigSpecOperationAdd,
					Device: &types.VirtualPCIPassthrough{
						VirtualDevice: types.VirtualDevice{
							Key: -20000,
							Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
								Vgpu: "mockup-vmiop",
							},
						},
					},
				},
			},
		},
	}
}

func CreateSpecE2eVMClassGPUnDDPIOConfigSpec() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:                VMClassGPUnDDPIOConfigSpec,
		CPUCount:          new(2),
		MemoryMB:          new(4096),
		CPUReservation:    new(100),
		MemoryReservation: new(100),
		ConfigSpec: &types.VirtualMachineConfigSpec{
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: types.VirtualDeviceConfigSpecOperationAdd,
					Device: &types.VirtualPCIPassthrough{
						VirtualDevice: types.VirtualDevice{
							Key: -20000,
							Backing: &types.VirtualPCIPassthroughVmiopBackingInfo{
								Vgpu: "mockup-vmiop",
							},
						},
					},
				},
				&types.VirtualDeviceConfigSpec{
					Operation: types.VirtualDeviceConfigSpecOperationAdd,
					Device: &types.VirtualPCIPassthrough{
						VirtualDevice: types.VirtualDevice{
							Key: -30000,
							Backing: &types.VirtualPCIPassthroughDynamicBackingInfo{
								AllowedDevice: []types.VirtualPCIPassthroughAllowedDevice{
									{
										VendorId: 52,
										DeviceId: 53,
									},
								},
								CustomLabel: "SampleLabel2",
							},
						},
					},
				},
			},
			ExtraConfig: []types.BaseOptionValue{
				&types.OptionValue{
					Key:   "hello-test-key",
					Value: "hello-test-value",
				},
			},
		},
	}
}

// CreateSpecE2eVMClassVTPMConfigSpec creates a vm class with a vTPM.
func CreateSpecE2eVMClassVTPMConfigSpec() wcp.VMClassSpec {
	return wcp.VMClassSpec{
		ID:       "e2etest-best-effort-small-with-vtpm",
		CPUCount: new(2),
		MemoryMB: new(4096),
		ConfigSpec: &types.VirtualMachineConfigSpec{
			DeviceChange: []types.BaseVirtualDeviceConfigSpec{
				&types.VirtualDeviceConfigSpec{
					Operation: types.VirtualDeviceConfigSpecOperationAdd,
					Device: &types.VirtualTPM{
						VirtualDevice: types.VirtualDevice{
							Key: -40000,
						},
					},
				},
			},
		},
	}
}

// EnsureVMClassPresent ensures the VM class exists in the WCP vcenter.
func EnsureVMClassPresent(wcpClient wcp.WorkloadManagementAPI, className string, params ...any) error {
	_, err := wcpClient.GetVMClassInfo(className)
	if err == nil {
		return nil
	}

	var dcliErr wcp.DcliError
	Expect(errors.As(err, &dcliErr)).Should(BeTrue())
	Expect(dcliErr.Response()).Should(ContainSubstring(VMClassNotFound))

	classSpec, err := GenerateVMClassSpecFunction(className, params...)
	Expect(err).ShouldNot(HaveOccurred())

	return wcpClient.CreateVMClass(classSpec.(wcp.VMClassSpec))
}

// EnsureVMClassAccess ensures the VM Class exists and the namespace has access to it.
func EnsureVMClassAccess(wcpClient wcp.WorkloadManagementAPI, className, namespace string) {
	Expect(EnsureVMClassPresent(wcpClient, className)).To(Succeed())
	Expect(EnsureNamespaceHasAccess(wcpClient, className, namespace)).To(Succeed())
}

// VerifyCLAssociation ensures that the namespace has access to expected content libraries.
func VerifyCLAssociation(wcpClient wcp.WorkloadManagementAPI, ns string, cls []string) {
	updateSpec := wcp.NamespaceUpdateVMserviceSpec{ContentLibraries: &cls}
	err := wcpClient.UpdateNamespaceVMServiceSpec(ns, updateSpec)
	Expect(err).ShouldNot(HaveOccurred())
	namespaceInfo, err := wcpClient.GetNamespace(ns)
	Expect(err).ShouldNot(HaveOccurred())

	expectedCls := namespaceInfo.VMServiceSpec.ContentLibraries
	Expect(VerifyExpectedCls(expectedCls, cls)).Should(BeTrue())
}

// VerifyExpectedCls checks whether all the contentLibraries in cls are present in expectedCls.
// expectedCls contains tkgCl in addition to required contentLibraries.
func VerifyExpectedCls(expectedCls []string, cls []string) bool {
	for _, cl := range cls {
		found := slices.Contains(expectedCls, cl)

		if !found {
			return false
		}
	}

	return true
}

// CheckCLDisassociation verifies that the namespace doesn't have access to given content libraries.
func CheckCLDisassociation(wcpClient wcp.WorkloadManagementAPI, ns string, cls []string) {
	namespaceInfo, err := wcpClient.GetNamespace(ns)
	Expect(err).ShouldNot(HaveOccurred())

	currentCls := namespaceInfo.VMServiceSpec.ContentLibraries
	Expect(VerifyDetachedCls(currentCls, cls)).Should(BeTrue())
}

// VerifyDetachedCls checks whether all the contentLibraries in detachedCls are not present in cls.
func VerifyDetachedCls(cls []string, detachedCls []string) bool {
	for _, deCl := range detachedCls {
		if slices.Contains(cls, deCl) {
			return false
		}
	}

	return true
}

func VerifyConfigMapCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, configMapName string) {
	By("Verify that we have a ConfigMap CRD")
	Eventually(func(g Gomega) {
		cm, err := utils.GetConfigMap(ctx, svClusterClient, ns, configMapName)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(cm).ShouldNot(BeNil())
	}, config.GetIntervals("default", "wait-config-map-creation")...).Should(Succeed(), "Timed out waiting for ConfigMap %s to be created in namespace %s", configMapName, ns)
}

func VerifySecretCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, secretName string) {
	By("Verify that we have a Secret CRD")
	Eventually(func(g Gomega) {
		cm, err := utils.GetSecret(ctx, svClusterClient, ns, secretName)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(cm).ShouldNot(BeNil())
	}, config.GetIntervals("default", "wait-secret-creation")...).Should(Succeed(), "Timed out waiting for Secret %s to be created in namespace %s", secretName, ns)
}

func VerifySecurityPolicyCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, sName string) {
	By("Verify that we have a Security Policy CRD")
	Eventually(func(g Gomega) {
		sp, err := utils.GetSecurityPolicy(ctx, svClusterClient, ns, sName)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(sp).ShouldNot(BeNil())
	}, config.GetIntervals("default", "wait-security-policy-creation")...).Should(Succeed(), "Timed out waiting for SecurityPolicy %s to be created in namespace %s", sName, ns)
}

func VerifySubnetOrSubnetSetCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, name, kind string) {
	By("Verify that we have a Subnet or SubnetSet CRD")
	Eventually(func(g Gomega) {
		switch kind {
		case "Subnet":
			subnet, err := utils.GetSubnet(ctx, svClusterClient, ns, name)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(subnet).ToNot(BeNil())
		case "SubnetSet":
			subnetSet, err := utils.GetSubnetSet(ctx, svClusterClient, ns, name)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(subnetSet).ToNot(BeNil())
		default:
			g.Expect(false).To(BeTrue(), "unknown kind: %s", kind)
		}
	}, config.GetIntervals("default", "wait-subnet-creation")...).Should(Succeed(), "Timed out waiting for %s %s to be created in namespace %s", kind, name, ns)
}

func VerifyWebConsoleRequestCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, webconsoleName string) {
	By("Verify that we have a WebConsoleRequest CRD")
	Eventually(func(g Gomega) {
		webconsole, err := utils.GetVWebConsoleRequest(ctx, svClusterClient, ns, webconsoleName)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(webconsole).ShouldNot(BeNil())
	}, config.GetIntervals("default", "wait-virtual-machine-web-console-request-creation")...).Should(Succeed(), "Timed out waiting for WebConsoleRequest %s to be created in namespace %s", webconsoleName, ns)
}

func VerifyVirtualMachineWebConsoleRequestCreation(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, vmWebconsoleName string) {
	By("Verify that we have a VirtualMachineWebConsoleRequest CRD")
	Eventually(func(g Gomega) {
		vmWebconsole, err := utils.GetVirtualMachineWebConsoleRequest(ctx, svClusterClient, ns, vmWebconsoleName)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(vmWebconsole).ShouldNot(BeNil())
	}, config.GetIntervals("default", "wait-virtual-machine-web-console-request-creation")...).Should(Succeed(), "Timed out waiting for VirtualMachineWebConsoleRequest %s to be created in namespace %s", vmWebconsoleName, ns)
}

func VerifyVMInZone(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, zone, vmName string) {
	By("Verify that VM is placed in zone")
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachine(ctx, svClusterClient, ns, vmName)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(vm.Status.Zone).Should(Equal(zone))
	}, config.GetIntervals("default", "wait-config-map-creation")...).Should(Succeed(), "Timed out waiting for VirtualMachine %s to be created")
}

// CreateLocalContentLibrary Utility function to create a local content library with given name.
func CreateLocalContentLibrary(clName string, wcpClient wcp.WorkloadManagementAPI) string {
	datastores, err := wcpClient.ListDatastores()
	Expect(err).NotTo(HaveOccurred(), "failed to list datastores")
	Expect(len(datastores)).NotTo(BeZero(), "no datastores found")

	// Choose the first datastore we see.
	dsForCL := datastores[0]
	clID, err := wcpClient.CreateLocalContentLibrary(clName, wcp.StorageBackingInfo{
		StorageBackings: []wcp.BackingInfo{
			{
				DatastoreID: dsForCL.Datastore,
				Type:        "DATASTORE",
			},
		},
	})
	Expect(err).NotTo(HaveOccurred(), "failed to create a new content library for publishing")
	Expect(clID).NotTo(BeEmpty(), "new publishing content library ID is empty")

	return clID
}

func GetK8sContentLibraryNameByUUID(ctx context.Context, config *config.E2EConfig, svClusterClient ctrlclient.Client, ns, contentLibraryID string) (string, error) {
	var k8sContentLibraryName string

	Eventually(func() (string, error) {
		k8sCLObj, err := utils.GetContentLibraryByUUID(ctx, svClusterClient, ns, contentLibraryID)
		if err != nil {
			return "", err
		}

		k8sContentLibraryName = k8sCLObj.Name

		return k8sContentLibraryName, nil
	}, config.GetIntervals("default", "wait-content-library-name")...).ShouldNot(BeEmpty(), "failed to get the k8s content library object by UUID: %s", contentLibraryID)

	return k8sContentLibraryName, nil
}

// GetContentLibraryUUIDByName returns the content library UUID by a given name.
func GetContentLibraryUUIDByName(clName string, wcpClient wcp.WorkloadManagementAPI) string {
	libraries, err := wcpClient.ListContentLibraries()
	Expect(err).NotTo(HaveOccurred())
	vmserviceCLID, err := wcpClient.FetchContentLibraryIDByName(clName, libraries)
	Expect(err).NotTo(HaveOccurred())
	Expect(vmserviceCLID).NotTo(BeEmpty())

	return vmserviceCLID
}

// Wait for a max of 2 minutes for the given pod to be ready else return an error.
func WaitForPodReady(ctx context.Context, config *config.E2EConfig, client ctrlclient.Client, ns, name string) {
	Eventually(func() bool {
		pod, err := utils.GetPod(ctx, client, ns, name)
		if err != nil {
			framework.Logf("retry due to: %v", err)
			return false
		}

		for i := range pod.Status.Conditions {
			if pod.Status.Conditions[i].Type == corev1.PodReady {
				if pod.Status.Conditions[i].Status == corev1.ConditionTrue {
					framework.Logf("Pod: %s, ready state: %v", pod.Name, corev1.PodReady)
					return true
				}
			}
		}

		return false
	}, config.GetIntervals("default", "wait-pod-ready")...).Should(BeTrue(), "Timed out waiting for Pod %s/%s to be ready", ns, name)
}

// VerifyLoginAndRunCmdsInNSXSetup verifies ssh login to a given VM's Ip address and run commands with
// corresponding expected outputs.
// For NSX setup, VMs will be inaccessible directly. NSX-t setup uses a PodVM as a jumpbox to create connection with VM.
func VerifyLoginAndRunCmdsInNSXSetup(ctx context.Context, config *config.E2EConfig, clusterProxy *common.VMServiceClusterProxy,
	namespace string, podVMName string, vmIP string, cmds []string, expectedOutput []string) {
	framework.Logf("will attempt to ssh into %s using jumpbox podvm", vmIP)

	Eventually(func() bool {
		stdout, err := clusterProxy.Exec(ctx, "-it", "jumpbox", "-n", namespace, "--", "sshpass", "-V")
		if err == nil && stdout != nil {
			return true
		}
		// The Exec function will output an error message on each failure.
		// Add a log here to clarify that retries are expected behavior.
		framework.Logf("sshpass not yet installed on jumpbox PodVM, retrying...")

		return false
	}, config.GetIntervals("default", "wait-jumpbox-sshpass-ready")...).Should(BeTrue(), "Timed out waiting for sshpass installation on jumpbox PodVM")

	framework.Logf("sshpass installed on jumpbox PodVM; verifying ssh login to VM with 'ip addr' command")

	sshHostField := fmt.Sprintf("%s@%s", consts.DefaultVMUserName, vmIP)
	Eventually(func(g Gomega) bool {
		_, err := clusterProxy.Exec(ctx, "-it", podVMName, "-n", namespace, "--", "rm", "-rf", "/root/.ssh/known_hosts")
		g.Expect(err).NotTo(HaveOccurred(), "failed to remove SSH known_hosts file in jumpbox PodVM")
		stdout, err := clusterProxy.Exec(ctx, "-it", podVMName, "-n", namespace, "--", "sshpass", "-p", consts.DefaultVMPassword, "ssh", "-o", "StrictHostKeyChecking no", sshHostField, "ip", "addr")
		g.Expect(err).NotTo(HaveOccurred(), "failed to SSH into VM to run 'ip addr' command")

		if strings.Contains(string(stdout), vmIP) {
			framework.Logf("'ip addr' command output:\n%s", string(stdout))
			return true
		}

		return false
	}, config.GetIntervals("default", "login-retry-timeout")...).Should(BeTrue(), "timeout SSH into VM or 'ip addr' command output does not contain VM IP %q", vmIP)

	framework.Logf("ssh login verified; running requested commands on VM")
	Expect(len(cmds)).To(Equal(len(expectedOutput)), "number of commands and expected outputs must be the same")

	for i, cmd := range cmds {
		framework.Logf("running cmd %q via jumpbox PodVM", cmd)
		Eventually(func(g Gomega) {
			_, err := clusterProxy.Exec(ctx, "-it", podVMName, "-n", namespace, "--", "rm", "-rf", "~/.ssh/known_hosts")
			g.Expect(err).NotTo(HaveOccurred())
			stdout, err := clusterProxy.Exec(ctx, "-it", podVMName, "-n", namespace, "--", "sshpass", "-p", consts.DefaultVMPassword, "ssh", "-o", "StrictHostKeyChecking no", sshHostField, cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(stdout)).To(ContainSubstring(expectedOutput[i]))
		}, config.GetIntervals("default", "login-retry-timeout")...).Should(Succeed(), "failed to run cmd %q or expected output %q not found from cmdOut", cmd, expectedOutput[i])
	}
}

// VerifyLoginAndRunCmdsInVDSSetup verifies ssh login to a given VM's Ip address and run commands with
// corresponding expected outputs.
// For VDS setup, VMs will be inaccessible directly,
// the gateway VM will be used as a jumpbox to create connection with VM.
func VerifyLoginAndRunCmdsInVDSSetup(config *config.E2EConfig, vmIP string, cmds []string, expectedOutput []string) {
	By(fmt.Sprintf("Verify that ssh login to VM with vmIP (ipv4): %s succeeds", vmIP))

	httpProxy := os.Getenv(consts.HTTPProxyEnv)
	Expect(httpProxy).NotTo(BeEmpty(), fmt.Sprintf("%s env var is not set", consts.HTTPProxyEnv))

	gatewayVMIP := strings.Split(httpProxy, ":")[0]

	gatewayCmdRunner, err := e2essh.NewSSHCommandRunner(gatewayVMIP, consts.SshPort, testbed.GatewayUsername, []ssh.AuthMethod{ssh.Password(testbed.GatewayPassword)})
	Expect(err).NotTo(HaveOccurred())
	Expect(gatewayCmdRunner).NotTo(BeNil())

	findTCPForwardingEntity := fmt.Sprintf("line=$(sed -n '/%s/=' %s | tail -1)", consts.AllowTCPForwardingKey, consts.SshdConfig)
	enableTCPForwarding := fmt.Sprintf("sed -i \"${line}s/no/yes/\" %s", consts.SshdConfig)
	enableTCPForwardingCmd := fmt.Sprintf("%s;%s;%s", findTCPForwardingEntity, enableTCPForwarding, consts.CmdRestartSSHD)
	_, err = gatewayCmdRunner.RunCommand(enableTCPForwardingCmd)
	Expect(err).NotTo(HaveOccurred())
	time.Sleep(10 * time.Second)

	gw := e2essh.Gateway{
		Hostname:    gatewayVMIP,
		Username:    testbed.GatewayUsername,
		Port:        consts.SshPort,
		AuthMethods: []ssh.AuthMethod{ssh.Password(testbed.GatewayPassword)},
	}

	var cmdRunner e2essh.SSHCommandRunner

	Eventually(func(g Gomega) {
		cmdRunner, err = e2essh.NewSSHCommandRunnerWithinGateway(vmIP, consts.SshPort, consts.DefaultVMUserName, []ssh.AuthMethod{ssh.Password(consts.DefaultVMPassword)}, gw)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(cmdRunner).ToNot(BeNil())
	}, config.GetIntervals("default", "login-retry-timeout")...).Should(Succeed(), "ssh login to VM did not succeed in time.")

	cmdOutput, err := cmdRunner.RunCommand("ip addr")
	Expect(err).NotTo(HaveOccurred())

	cmdOutputString := string(cmdOutput)
	Expect(cmdOutputString).To(ContainSubstring(vmIP))

	if len(cmds) > 0 && len(expectedOutput) > 0 {
		for i, cmd := range cmds {
			By(fmt.Sprintf("Verify running cmd: %s on VM with vmIP (ipv4): %s contains expected output: %s", cmd, vmIP, expectedOutput[i]))
			cmdOutput, err = cmdRunner.RunCommand(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmdOutputString = string(cmdOutput)
			Expect(cmdOutputString).To(ContainSubstring(expectedOutput[i]))
		}
	}

	disableForwarding := fmt.Sprintf("sed -i \"${line}s/yes/no/\" %s", consts.SshdConfig)
	disableForwardingCmd := fmt.Sprintf("%s;%s;%s", findTCPForwardingEntity, disableForwarding, consts.CmdRestartSSHD)
	_, err = gatewayCmdRunner.RunCommand(disableForwardingCmd)
	Expect(err).NotTo(HaveOccurred())
}

// decodeGzipBase64 decodes a gzip-compressed and base64-encoded string.
func decodeGzipBase64(encoded string) (string, error) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Decompress gzip
	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}

	defer func() { _ = reader.Close() }()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to decompress gzip: %w", err)
	}

	return string(decompressed), nil
}

// WaitForBackupToComplete waits for the VM backup process to complete by verifying
// that all PVCs in the VM spec have corresponding entries in the backup data stored
// in the VM's ExtraConfig. This is exported for use in tests that perform in-place restores.
func WaitForBackupToComplete(
	ctx context.Context,
	vm *vmopv1a2.VirtualMachine,
	clusterProxy *common.VMServiceClusterProxy,
	config *config.E2EConfig,
) {
	waitForBackupToComplete(ctx, vm, clusterProxy, config)
}

// waitForBackupToComplete waits for the VM backup process to complete by verifying
// that all PVCs in the VM spec have corresponding entries in the backup data stored
// in the VM's ExtraConfig.
func waitForBackupToComplete(
	ctx context.Context,
	vm *vmopv1a2.VirtualMachine,
	clusterProxy *common.VMServiceClusterProxy,
	config *config.E2EConfig,
) {
	By("Waiting for backup to complete for all PVCs")

	// Get list of PVC names from VM spec
	expectedPVCNames := make(map[string]struct{})

	for _, vol := range vm.Spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			expectedPVCNames[vol.PersistentVolumeClaim.ClaimName] = struct{}{}
		}
	}

	// If there are no PVCs, no need to wait
	if len(expectedPVCNames) == 0 {
		framework.Logf("No PVCs found in VM spec, skipping backup wait")
		return
	}

	framework.Logf("Expected PVCs to be backed up: %v", strings.Join(slices.Collect(maps.Keys(expectedPVCNames)), ", "))

	// Get vCenter client
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vCenterClient)

	vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}
	propCollector := property.DefaultCollector(vCenterClient)

	// Wait for backup to complete
	Eventually(func() bool {
		var vmMO mo.VirtualMachine

		err := propCollector.RetrieveOne(ctx, vmMoRef, []string{"config.extraConfig"}, &vmMO)
		if err != nil {
			framework.Logf("Failed to retrieve VM ExtraConfig: %v", err)
			return false
		}

		// Check if Config is populated
		if vmMO.Config == nil {
			framework.Logf("VM Config is nil")
			return false
		}

		// Check if PVC disk data exists in ExtraConfig
		ecList := object.OptionValueList(vmMO.Config.ExtraConfig)

		pvcDiskDataEncoded, found := ecList.GetString(PVCDiskDataExtraConfigKey)
		if !found {
			framework.Logf("PVC disk data not found in ExtraConfig yet")
			return false
		}

		// Decode the backup data
		pvcDiskDataJSON, err := decodeGzipBase64(pvcDiskDataEncoded)
		if err != nil {
			framework.Logf("Failed to decode PVC disk data: %v", err)
			return false
		}

		// Parse the JSON to get list of PVCDiskData
		var pvcDiskDataList []PVCDiskData
		if err := json.Unmarshal([]byte(pvcDiskDataJSON), &pvcDiskDataList); err != nil {
			framework.Logf("Failed to unmarshal PVC disk data: %v", err)
			return false
		}

		// Check if all expected PVCs are in the backup
		backedUpPVCs := make(map[string]struct{})
		for _, pvcData := range pvcDiskDataList {
			backedUpPVCs[pvcData.PVCName] = struct{}{}
		}

		framework.Logf("Backed up PVCs: %v", strings.Join(slices.Collect(maps.Keys(backedUpPVCs)), ", "))

		// Verify all expected PVCs are backed up
		for pvcName := range expectedPVCNames {
			if _, found := backedUpPVCs[pvcName]; !found {
				framework.Logf("PVC %s not yet backed up", pvcName)
				return false
			}
		}

		framework.Logf("All PVCs have been backed up successfully")

		return true
	}, config.GetIntervals("default", "wait-backup-to-complete")...).Should(BeTrue(), "backup did not complete for all PVCs")
}

// UnregisterPVCVolumes unregisters all PVCs in the provided list using CnsUnregisterVolume.
// This simulates a backup/restore scenario where the VM on vCenter still has its disks,
// but the Kubernetes PVC/PV objects are gone.
func UnregisterPVCVolumes(
	ctx context.Context,
	svClusterClient ctrlclient.Client,
	namespace string,
	vmName string,
	pvcNames []string,
	config *config.E2EConfig,
) {
	By("Unregister volumes using CnsUnregisterVolume for each PVC")

	// Create CnsUnregisterVolume resource for each PVC
	for _, pvcName := range pvcNames {
		framework.Logf("Creating CnsUnregisterVolume for PVC: %s", pvcName)

		// Generate a unique name for the CnsUnregisterVolume resource
		unregisterName := fmt.Sprintf("%s-unreg-%s", vmName, capiutil.RandomString(6))

		// Create the CnsUnregisterVolume object using the typed API
		cnsUnregister := &cnsunregistervolumev1alpha1.CnsUnregisterVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      unregisterName,
				Namespace: namespace,
			},
			Spec: cnsunregistervolumev1alpha1.CnsUnregisterVolumeSpec{
				PVCName:         pvcName,
				RetainFCD:       false,
				ForceUnregister: true,
			},
		}

		// Create the CnsUnregisterVolume resource
		Expect(svClusterClient.Create(ctx, cnsUnregister)).To(Succeed(), "failed to create CnsUnregisterVolume for PVC %s", pvcName)
		framework.Logf("Successfully created CnsUnregisterVolume: %s for PVC: %s", unregisterName, pvcName)

		// Wait for the CnsUnregisterVolume status to show unregistered: true
		Eventually(func() bool {
			cnsUnregisterStatus := &cnsunregistervolumev1alpha1.CnsUnregisterVolume{}

			err := svClusterClient.Get(ctx, ctrlclient.ObjectKey{
				Namespace: namespace,
				Name:      unregisterName,
			}, cnsUnregisterStatus)
			if err != nil {
				framework.Logf("Error getting CnsUnregisterVolume %s: %v", unregisterName, err)
				return false
			}

			return cnsUnregisterStatus.Status.Unregistered
		}, config.GetIntervals("default", "wait-pvc-deletion")...).Should(BeTrue(), "CnsUnregisterVolume %s should have status.unregistered=true", unregisterName)

		framework.Logf("PVC %s has been successfully unregistered (CnsUnregisterVolume status confirmed)", pvcName)

		// Optionally verify the PVC is deleted (commented out - relying on CnsUnregisterVolume status instead)
		// TODO: VMSVC-3346: Investigate why PV / PVCs are not being cleaned up.
		// Eventually(func() bool {
		// 	pvc := &corev1.PersistentVolumeClaim{}
		// 	pvcKey := ctrlclient.ObjectKey{
		// 		Namespace: namespace,
		// 		Name:      pvcName,
		// 	}
		// 	err := svClusterClient.Get(ctx, pvcKey, pvc)
		// 	return apierrors.IsNotFound(err)
		// }, config.GetIntervals("default", "wait-pvc-deletion")...).Should(BeTrue(), "PVC %s should be deleted by CnsUnregisterVolume", pvcName)
	}
}

// DeleteVMResource will delete only the K8s VM to simulate a restored VM in vSphere.
// It uses CnsUnregisterVolume to remove PVCs while keeping disks attached to the VM.
func DeleteVMResource(
	ctx context.Context,
	vmName, vmNamespace string,
	bootstrapResourceYAML []byte,
	clusterProxy *common.VMServiceClusterProxy,
	config *config.E2EConfig,
	svClusterClient ctrlclient.Client) string {
	By("Get VM before powering off")

	vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmNamespace, vmName)
	Expect(err).ToNot(HaveOccurred())

	// Wait for backup to complete before powering off and deleting the VM
	waitForBackupToComplete(ctx, vm, clusterProxy, config)

	By("Power off the VM")
	vmoperator.UpdateVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, string(vmopv1a2.VirtualMachinePowerStateOff))
	vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, string(vmopv1a2.VirtualMachinePowerStateOff))

	By("Add the pause annotation to VM")

	vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmNamespace, vmName)
	Expect(err).ToNot(HaveOccurred())

	if vm.Annotations == nil {
		vm.Annotations = make(map[string]string)
	}

	vm.Annotations[vmopv1a2.PauseAnnotation] = trueString
	Expect(svClusterClient.Update(ctx, vm)).To(Succeed())

	// Collect all PVC names from the VM spec
	var pvcNames []string

	for _, volume := range vm.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	// Unregister all PVCs using the helper function
	UnregisterPVCVolumes(ctx, svClusterClient, vmNamespace, vmName, pvcNames, config)

	By("Delete only the K8s VM (vSphere VM should remain with the paused annotation applied)")
	vmoperator.DeleteVirtualMachine(ctx, svClusterClient, vmNamespace, vmName)

	// The finalizer should be removed after the VM is being deleted to avoid
	// its being added again during the normal reconciliation by the controller.
	By("Remove the VMOP finalizer to ensure deletion of K8s VM")
	Eventually(func() bool {
		vm, err = utils.GetVirtualMachine(ctx, svClusterClient, vmNamespace, vmName)
		if apierrors.IsNotFound(err) {
			// VM is already deleted, nothing to do.
			return true
		}

		if err != nil {
			return false
		}

		if vm.DeletionTimestamp.IsZero() {
			err = fmt.Errorf("VM %s/%s does not have deletion timestamp set", vmNamespace, vmName)
			return false
		}

		controllerutil.RemoveFinalizer(vm, VMFinalizerName)
		// Also remove the deprecated finalizer if it exists to ensure backward compatibility.
		controllerutil.RemoveFinalizer(vm, VMFinalizerNameDeprecated)
		err = svClusterClient.Update(ctx, vm)

		return err == nil
	}, 30*time.Second, 3*time.Second).Should(BeTrue(), "failed to remove finalizer from VM '%s/%s', most recent error: %v", vmNamespace, vmName, err)

	vmoperator.WaitForVirtualMachineToBeDeleted(ctx, config, svClusterClient, vmNamespace, vmName)

	if bootstrapResourceYAML != nil {
		By("Delete the bootstrap resource")
		Expect(clusterProxy.DeleteWithArgs(ctx, bootstrapResourceYAML)).To(Succeed(), "failed to delete VM bootstrap resource")
	}

	return vm.Status.UniqueID
}

// InvokeRegisterVM invokes the RegisterVM API.
// Waits for the Task to complete, returning TaskInfo result.
func InvokeRegisterVM(
	ctx context.Context,
	vmMoID, vmNamespace string,
	clusterProxy *common.VMServiceClusterProxy,
	wcpClient wcp.WorkloadManagementAPI) (*types.TaskInfo, error) {
	By("Invoke the RegisterVM API")

	taskID, err := wcpClient.RegisterVM(vmNamespace, vmMoID)
	Expect(err).ToNot(HaveOccurred())
	Expect(taskID).ToNot(BeEmpty())

	By("Wait for the registerVM API task to complete")

	taskID = fromVmodl1ID(taskID)
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	taskMoref := types.ManagedObjectReference{Type: "Task", Value: taskID}
	task := object.NewTask(vCenterClient, taskMoref)

	return task.WaitForResult(ctx, nil)
}

// VerifyPostRegisterVM verifies expected VM state after a successful call to RegisterVM.
func VerifyPostRegisterVM(
	ctx context.Context,
	vmName, vmNamespace string,
	bootstrapResourceYAML []byte,
	expectedRestoredPVCCount int,
	clusterProxy *common.VMServiceClusterProxy,
	config *config.E2EConfig,
	svClusterClient ctrlclient.Client,
	wcpClient wcp.WorkloadManagementAPI) {
	By("Verify that the VM has been created in PoweredOff state")
	vmoperator.WaitForVirtualMachineToExist(ctx, config, svClusterClient, vmNamespace, vmName)
	vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, string(vmopv1a2.VirtualMachinePowerStateOff))

	vm, err := utils.GetVirtualMachine(ctx, svClusterClient, vmNamespace, vmName)
	Expect(err).ToNot(HaveOccurred())

	By("Restored VM must have expected number of restored volumes")

	actualRestoredPVCCount := 0

	for _, vol := range vm.Spec.Volumes {
		// Volume and PVC both contain "restored-" prefix, so validate that.
		if vol.PersistentVolumeClaim != nil &&
			strings.HasPrefix(vol.PersistentVolumeClaim.ClaimName, "restored-") {
			actualRestoredPVCCount++
		}
	}

	Expect(actualRestoredPVCCount).To(Equal(expectedRestoredPVCCount), "Restored VM must have expected number of restored volumes, expected %d, got %d",
		expectedRestoredPVCCount, actualRestoredPVCCount)

	By("Power on the VM")
	vmoperator.UpdateVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, string(vmopv1a2.VirtualMachinePowerStateOn))
	vmoperator.WaitForVirtualMachinePowerState(ctx, config, svClusterClient, vmNamespace, vmName, string(vmopv1a2.VirtualMachinePowerStateOn))

	By("Verify that the VM has an IP assigned")
	vmoperator.WaitForVirtualMachineIP(ctx, config, svClusterClient, vmNamespace, vmName)
	restoredVMIp := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, vmNamespace, vmName)
	framework.Logf("restored vm ip is: %s ", restoredVMIp)

	By("Verify that the restored VM IP matches the IP in the corresponding network provider resource")

	newIP := vmoperator.GetVirtualMachineNetworkProviderIP(ctx, config, svClusterClient, vmNamespace, vmName)
	// In some cases, a restored VM could keep old IP until GOSC runs again to update the guest network with new IP.
	// Wait for the VM to have the new IP set in the guest network.
	Eventually(func() bool {
		vmIP := vmoperator.GetVirtualMachineIP(ctx, svClusterClient, vmNamespace, vmName)
		return vmIP == newIP
	}, config.GetIntervals("default", "wait-virtual-machine-vmip")...).Should(BeTrue(), fmt.Sprintf("restored VM doesn't have the new IP from network provider: %s", newIP))
}

// VerifyRegisterVMOnlyClassicDisk verifies the register VM API with the following steps:
// - Power off the VM and delete only the K8s VM to simulate a restored VM in vSphere.
// - Remove the bootstrap resource if provided.
// - Invoke the RegisterVM API to register the VM on Supervisor.
// - Verify that the VM is created in poweredOff state.
// - Remove the paused annotation that was added earlier to ensure the VM is fully reconciled.
// - Power on the VM and verify that the VM has an IPV4 assigned.
// - Verify that the VM IP matches the IP in the VM's network provider resource.
// The source VM only has classic disk, no PVCs.
func VerifyRegisterVMOnlyClassicDisk(
	ctx context.Context,
	vmName, vmNamespace string,
	bootstrapResourceYAML []byte,
	clusterProxy *common.VMServiceClusterProxy,
	config *config.E2EConfig,
	svClusterClient ctrlclient.Client,
	wcpClient wcp.WorkloadManagementAPI) {
	By("Verifying Register VM...")

	vmMoID := DeleteVMResource(ctx, vmName, vmNamespace, bootstrapResourceYAML, clusterProxy, config, svClusterClient)

	taskInfo, err := InvokeRegisterVM(ctx, vmMoID, vmNamespace, clusterProxy, wcpClient)

	By("Verify task state is success")
	Expect(err).ToNot(HaveOccurred())
	Expect(taskInfo).ToNot(BeNil())
	Expect(taskInfo.Error).To(BeNil())
	Expect(taskInfo.State).To(Equal(types.TaskInfoStateSuccess))

	// We only expect one restored volume that will be the classic disk
	// converted to a PVC because of all disks PVC.
	expectedRestoredPVCCount := 1
	VerifyPostRegisterVM(
		ctx,
		vmName,
		vmNamespace,
		bootstrapResourceYAML,
		expectedRestoredPVCCount,
		clusterProxy,
		config,
		svClusterClient,
		wcpClient,
	)

	By("Finish verifying Register VM")
}

// fromVmodl1ID given Vmodl1 identifier for a managed object, returns its MoID.
// Taken from WCP service.
func fromVmodl1ID(id string) string {
	if ix := strings.LastIndex(id, ":"); ix > 0 {
		return id[0:ix]
	}
	// TODO: The function should return empty string on error.
	// Returning the original ID till VKAL-2139 is resolved.
	// Also, we need to return error in this case.
	return id
}

func LabelVM(ctx context.Context, config *config.E2EConfig, clusterProxy *common.VMServiceClusterProxy, vmName, namespace, labelKey, labelVal string) error {
	framework.Logf("Labeling VM %s with %s=%s", vmName, labelKey, labelVal)

	label := fmt.Sprintf("%s=%s", labelKey, labelVal)

	err := clusterProxy.Label(ctx, "vm", vmName, "-n", namespace, label)
	if err != nil {
		return fmt.Errorf("failed to label VM %s: %w", vmName, err)
	}

	return nil
}

// DeployVMWithCloudInit deploys a VM with the default cloud-init config passed.
func DeployVMWithCloudInit(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	clusterResources *config.Resources,
	ns, vmName, groupName string,
	pvcs []manifestbuilders.PVC) {
	secretName := "cloud-config-data-" + capiutil.RandomString(4)
	secret := manifestbuilders.Secret{
		Namespace: ns,
		Name:      secretName,
	}
	secretYaml := manifestbuilders.GetSecretYamlCloudConfig(secret)
	Expect(vmSvcClusterProxy.CreateWithArgs(ctx, secretYaml)).To(Succeed(), "failed to create the Secret with cloud-config data", string(secretYaml))

	linuxImageDisplayName := GetDefaultImageDisplayName(clusterResources)

	vmParameters := manifestbuilders.VirtualMachineYaml{
		Namespace:        ns,
		Name:             vmName,
		ImageName:        linuxImageDisplayName,
		VMClassName:      clusterResources.VMClassName,
		StorageClassName: clusterResources.StorageClassName,
		SecretName:       secretName,
		PVCs:             pvcs,
	}
	if groupName != "" {
		vmParameters.GroupName = groupName
	}

	vmYaml := manifestbuilders.GetVirtualMachineYamlA5(vmParameters)
	framework.Logf("Create VirtualMachine:\n%s", string(vmYaml))
	Expect(vmSvcClusterProxy.CreateWithArgs(ctx, vmYaml)).To(Succeed(), "failed to create VM:\n%s", string(vmYaml))
}

func CreateVMGroup(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	params manifestbuilders.VirtualMachineGroupYaml,
) {
	vmGroupYaml := manifestbuilders.GetVirtualMachineGroupYaml(params)
	framework.Logf("Create VirtualMachineGroup:\n%s", string(vmGroupYaml))
	Expect(vmSvcClusterProxy.CreateWithArgs(ctx, vmGroupYaml)).To(Succeed())
}

func DeleteVMGroup(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	vmSvcE2EConfig *config.E2EConfig,
	params manifestbuilders.VirtualMachineGroupYaml,
) {
	vmGroupYaml := manifestbuilders.GetVirtualMachineGroupYaml(params)
	framework.Logf("Delete VirtualMachineGroup:\n%s", string(vmGroupYaml))
	Expect(vmSvcClusterProxy.Delete(ctx, vmGroupYaml)).To(Succeed())

	Eventually(func() bool {
		_, err := utils.GetVirtualMachineGroup(ctx, vmSvcClusterProxy.GetClient(), params.Namespace, params.Name)
		return apierrors.IsNotFound(err)
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-group-deletion")...).To(BeTrue())
}

func CreateVMGroupPub(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	params manifestbuilders.VirtualMachineGroupPublishRequestYaml,
	errMsg string,
) {
	vmGroupPubYaml := manifestbuilders.GetVirtualMachineGroupPublishRequestYaml(params)
	framework.Logf("Create VirtualMachineGroupPublishRequest:\n%s", string(vmGroupPubYaml))

	if len(errMsg) == 0 {
		Expect(vmSvcClusterProxy.CreateWithArgs(ctx, vmGroupPubYaml)).To(Succeed())
	} else {
		_, stderr, err := vmSvcClusterProxy.CreateRawWithArgs(ctx, vmGroupPubYaml)
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring(errMsg))
	}
}

func UpdateVMGroupPub(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	params manifestbuilders.VirtualMachineGroupPublishRequestYaml) {
	vmGroupPubYaml := manifestbuilders.GetVirtualMachineGroupPublishRequestYaml(params)
	framework.Logf("Update VirtualMachineGroupPublishRequest:\n%s", string(vmGroupPubYaml))
	Expect(vmSvcClusterProxy.ApplyWithArgs(ctx, vmGroupPubYaml)).To(Succeed())
}

func CreateVMSnapshot(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	params manifestbuilders.VirtualMachineSnapshotYaml,
) {
	vmSnapshotYaml := manifestbuilders.GetVirtualMachineSnapshotYaml(params)
	framework.Logf("Create VirtualMachineSnapshot:\n%s", string(vmSnapshotYaml))
	Expect(vmSvcClusterProxy.CreateWithArgs(ctx, vmSnapshotYaml)).To(Succeed())
}

func CreateSnapshotInVC(
	ctx context.Context,
	clusterProxy *common.VMServiceClusterProxy,
	vmSnapshotName string,
	vmName string,
	vmNamespace string,
) {
	vCenterClient := vcenter.NewVimClientFromKubeconfig(ctx, clusterProxy.GetKubeconfigPath())
	defer vcenter.LogoutVimClient(vCenterClient)

	vm, err := utils.GetVirtualMachineA5(ctx, clusterProxy.GetClient(), vmNamespace, vmName)
	Expect(err).NotTo(HaveOccurred())

	vmMoID := vm.Status.UniqueID
	vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID}
	vmObj := object.NewVirtualMachine(vCenterClient, vmMoRef)

	task, err := vmObj.CreateSnapshot(ctx, vmSnapshotName, "description", true, true)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	_, err = task.WaitForResult(ctx)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	var vmMO mo.VirtualMachine

	propCollector := property.DefaultCollector(vCenterClient)
	ExpectWithOffset(1, propCollector.RetrieveOne(ctx, vmMoRef, []string{"snapshot"}, &vmMO)).To(Succeed())
	ExpectWithOffset(1, vmMO.Snapshot).ToNot(BeNil())
}

func UpdateVMSnapshot(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	vmSvcE2EConfig *config.E2EConfig,
	params manifestbuilders.VirtualMachineSnapshotYaml,
) {
	vmSnapshotYaml := manifestbuilders.GetVirtualMachineSnapshotYaml(params)
	framework.Logf("Update VirtualMachineSnapshot:\n%s", string(vmSnapshotYaml))
	Eventually(func() error {
		return vmSvcClusterProxy.ApplyWithArgs(ctx, vmSnapshotYaml)
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-snapshot-update")...).Should(Succeed())
}

func RevertVMSnapshot(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	vmSvcE2EConfig *config.E2EConfig,
	vmName, vmNamespace string,
	currentSnapshot *vmopv1a5.VirtualMachineSnapshotReference,
) {
	GinkgoHelper()

	Expect(currentSnapshot.Name).NotTo(BeEmpty())

	vm, err := utils.GetVirtualMachineA5(ctx, vmSvcClusterProxy.GetClient(), vmNamespace, vmName)
	Expect(err).NotTo(HaveOccurred())

	vmPatch := vm.DeepCopy()
	vmPatch.Spec.CurrentSnapshotName = currentSnapshot.Name
	Expect(vmSvcClusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed())
	framework.Logf("Revert to VirtualMachineSnapshot:\n%s", currentSnapshot.Name)
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA5(ctx, vmSvcClusterProxy.GetClient(), vmNamespace, vmName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Status.CurrentSnapshot).To(Equal(currentSnapshot))
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-snapshot-revert")...).Should(Succeed())
}

func UpdateVMRestartMode(
	ctx context.Context,
	vmSvcClusterProxy *common.VMServiceClusterProxy,
	vmSvcE2EConfig *config.E2EConfig,
	vmName, vmNamespace string,
	restartMode vmopv1a5.VirtualMachinePowerOpMode,
) {
	GinkgoHelper()

	vm, err := utils.GetVirtualMachineA5(ctx, vmSvcClusterProxy.GetClient(), vmNamespace, vmName)
	Expect(err).NotTo(HaveOccurred())

	vmPatch := vm.DeepCopy()
	vmPatch.Spec.RestartMode = restartMode
	Expect(vmSvcClusterProxy.GetClient().Patch(ctx, vmPatch, ctrlclient.MergeFrom(vm))).To(Succeed())
	framework.Logf("Update VM RestartMode:\n%s", restartMode)
	Eventually(func(g Gomega) {
		vm, err := utils.GetVirtualMachineA5(ctx, vmSvcClusterProxy.GetClient(), vmNamespace, vmName)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vm.Spec.RestartMode).To(Equal(restartMode))
	}, vmSvcE2EConfig.GetIntervals("default", "wait-virtual-machine-restart-mode-update")...).Should(Succeed())
}

func GetDefaultImageDisplayName(clusterResources *config.Resources) string {
	if os.Getenv("RUN_CANONICAL_TEST") == "true" {
		Expect(clusterResources.UbuntuImageDisplayName).ToNot(BeEmpty(), "Invalid argument. UbuntuImageDisplayName can't be empty")
		return clusterResources.UbuntuImageDisplayName
	} else {
		Expect(clusterResources.PhotonImageDisplayName).ToNot(BeEmpty(), "Invalid argument. PhotonImageDisplayName can't be empty")
		return clusterResources.PhotonImageDisplayName
	}
}

// GetDefaultImageGuestID returns the guest ID for the default linux image.
func GetDefaultImageGuestID() string {
	if os.Getenv("RUN_CANONICAL_TEST") == "true" {
		return "ubuntu64Guest"
	}

	return "vmwarePhoton64Guest"
}

// VerifyVMTagsAndPolicyAssignment verifies the expected tags and policies assigned to the VM.
func VerifyVMTagsAndPolicyAssignment(
	ctx context.Context,
	config *config.E2EConfig,
	client ctrlclient.Client,
	mgr *tags.Manager,
	ns, vmName string,
	policyNameToTagID map[string]string,
	expectedPolicyNames []string) {
	GinkgoHelper()

	expectedTagIDs := make([]string, len(expectedPolicyNames))
	for i, policyName := range expectedPolicyNames {
		expectedTagIDs[i] = policyNameToTagID[policyName]
	}

	Eventually(func(g Gomega) {
		// Verify the K8s VM CR has the expected policies in status.
		vm, err := utils.GetVirtualMachineA5(ctx, client, ns, vmName)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get K8s VM CR")

		vmStatusPolicyNames := make([]string, len(vm.Status.Policies))
		for i, policy := range vm.Status.Policies {
			vmStatusPolicyNames[i] = policy.Name
		}

		g.Expect(vmStatusPolicyNames).To(ConsistOf(expectedPolicyNames))

		// Verify the vCenter VM has the expected tags assigned.
		g.Expect(vm.Status.UniqueID).NotTo(BeEmpty(), "VM unique ID should be present")
		vmMoRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: vm.Status.UniqueID}
		list, err := mgr.ListAttachedTags(ctx, vmMoRef)
		g.Expect(err).NotTo(HaveOccurred(), "failed to list attached tags for VM %s", vmMoRef.Value)
		g.Expect(list).To(ConsistOf(expectedTagIDs))
	}, config.GetIntervals("default", "wait-virtual-machine-condition-update")...).Should(Succeed())
}

func getThumbprint(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}

	conn, err := tls.Dial("tcp", host, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "", err
	}

	defer func() { _ = conn.Close() }()

	cert := conn.ConnectionState().PeerCertificates[0]
	hash := sha1.Sum(cert.Raw)

	hexHash := make([]string, 0, len(hash))
	for _, b := range hash {
		hexHash = append(hexHash, fmt.Sprintf("%02X", b))
	}

	return strings.Join(hexHash, ":"), nil
}

// EnsureVMServiceContentLibrary ensures that the vmservice content library exists.
func EnsureVMServiceContentLibrary(ctx context.Context, wcpClient wcp.WorkloadManagementAPI, subscriptionURL string) string {
	clName := consts.VMServiceCLName
	libraries, err := wcpClient.ListContentLibraries()
	Expect(err).NotTo(HaveOccurred())

	clID, err := wcpClient.FetchContentLibraryIDByName(clName, libraries)
	if err == nil && clID != "" {
		e2eframework.Byf("Content library %q already exists with ID %q", clName, clID)
		return clID
	}

	e2eframework.Byf("Creating VM Service content library %q", clName)

	datastores, err := wcpClient.ListDatastores()
	Expect(err).NotTo(HaveOccurred())

	var datastoreID string

	for _, dsName := range []string{"vsanDatastore", "sharedVmfs-0", "nfs0-1"} {
		for _, ds := range datastores {
			if ds.Name == dsName {
				datastoreID = ds.Datastore
				break
			}
		}

		if datastoreID != "" {
			break
		}
	}

	Expect(datastoreID).NotTo(BeEmpty(), "Failed to find a suitable datastore for content library")

	storageBackings := wcp.StorageBackingInfo{
		StorageBackings: []wcp.BackingInfo{
			{
				DatastoreID: datastoreID,
				Type:        "DATASTORE",
			},
		},
	}

	thumbprint, err := getThumbprint(subscriptionURL)
	if err != nil {
		framework.Logf("Warning: failed to get thumbprint for %s: %v", subscriptionURL, err)
	}

	clID, err = wcpClient.CreateSubscribedContentLibrary(clName, subscriptionURL, thumbprint, true, storageBackings)
	Expect(err).NotTo(HaveOccurred(), "Failed to create subscribed content library")

	e2eframework.Byf("Created content library %q with ID %q", clName, clID)

	// Wait for sync
	e2eframework.Byf("Waiting for VMService Content library synchronization")

	err = wcpClient.SyncSubscribedContentLibrary(clID)
	Expect(err).NotTo(HaveOccurred(), "Failed to sync subscribed content library")

	// Wait for the library to be synced
	Eventually(func() bool {
		items, err := wcpClient.ListContentLibraryItems(clID)
		if err != nil {
			return false
		}

		return len(items) > 0
	}, "5m", "10s").Should(BeTrue(), "Content library items should be synced")

	e2eframework.Byf("VMService Content library synchronization finished")

	return clID
}

const PVCDiskDataExtraConfigKey = "vmservice.virtualmachine.pvc.disk.data"

type PVCDiskData struct {
	FileName    string
	PVCName     string
	AccessModes []string
	UUID        string
}

// ESXConfig contains configuration for ESX host operations.
type ESXConfig struct {
	HostIPs  []string
	Username string
	Password string
	Build    string
}

// RBACRole defines a custom cluster role for e2e testing.
type RBACRole struct {
	Name     string
	APIGroup string
	Resource string
	Verbs    string
}

var (
	// vGPUConfiguredHosts tracks which hosts have already been configured to avoid duplicate work.
	vGPUConfiguredHosts = make(map[string]bool)
)

// EnsureVGPUConfiguration configures vGPUs on ESX hosts if needed for tests that require vGPU functionality.
// This function is idempotent - it tracks which hosts have been configured and skips already configured hosts.
// This replaces the Python logic from esx_helper.py that was used in gce2e_prerequisite.py.
//
// Usage: Call this function in BeforeEach or at the start of tests that need vGPU functionality.
func EnsureVGPUConfiguration(config ESXConfig) error {
	// Check if we should skip vGPU configuration
	if config.Username == "" || config.Password == "" || config.Build == "" {
		framework.Logf("Skipping vGPU configuration due to missing ESX credentials or build information")
		return nil
	}

	if strings.Contains(os.Getenv("TEST_SKIP"), "vGPU") {
		framework.Logf("Skipping vGPU configuration due to vGPU in TEST_SKIP environment variable")
		return nil
	}

	// Parse build number to construct VIB path
	buildParts := strings.Split(config.Build, "-")
	if len(buildParts) != 2 {
		return fmt.Errorf("invalid build format %s, expected format: <type>-<number>", config.Build)
	}

	buildType := buildParts[0]
	buildNumber := buildParts[1]

	// Convert build type (ob -> bora, keep sb as sb)
	if buildType == "ob" {
		buildType = "bora"
	}

	vibPath := fmt.Sprintf("http://build-squid.vcfd.broadcom.net/build/mts/release/%s-%s/publish/test-vmx.vib", buildType, buildNumber)

	// Commands to run on each ESX host
	commands := []string{
		fmt.Sprintf("esxcli software vib install -v %s", vibPath),
		"esxcli graphics host refresh",
	}

	// Configure vGPUs on each ESX host (skip if already configured)
	for _, hostIP := range config.HostIPs {
		// Check if this host was already configured
		if vGPUConfiguredHosts[hostIP] {
			framework.Logf("vGPU already configured on ESX host %s, skipping", hostIP)
			continue
		}

		e2eframework.Byf("Configuring vGPUs on ESX host: %s", hostIP)

		// Create SSH connection to the ESX host
		authMethods := []ssh.AuthMethod{ssh.Password(config.Password)}

		cmdRunner, err := e2essh.NewSSHCommandRunner(hostIP, 22, config.Username, authMethods)
		if err != nil {
			framework.Logf("Warning: Failed to connect to ESX host %s: %v", hostIP, err)
			continue
		}

		// Execute commands on the ESX host
		hostConfigured := true

		for _, cmd := range commands {
			framework.Logf("Running command on ESX host %s: %s", hostIP, cmd)

			_, err := cmdRunner.RunCommand(cmd)
			if err != nil {
				// Log warning but don't fail - this is best effort
				// Not all ESX hosts need to have vGPU installed
				framework.Logf("Warning: Failed to run command '%s' on ESX host %s: %v", cmd, hostIP, err)

				hostConfigured = false
			}
		}

		// Mark host as configured if all commands succeeded
		if hostConfigured {
			vGPUConfiguredHosts[hostIP] = true
			framework.Logf("Successfully configured vGPUs on ESX host: %s", hostIP)
		}
	}

	return nil
}

// ParseESXHosts parses comma-separated ESX host IPs into a slice.
func ParseESXHosts(hostIPs string) []string {
	if hostIPs == "" {
		return nil
	}

	return strings.Split(hostIPs, ",")
}

// NewESXConfigFromEnv creates an ESXConfig from environment variables.
// Returns nil if required environment variables are not set.
// Environment variables:
//   - ESX_IPS: Comma-separated list of ESX host IPs
//   - ESX_USER: Username for ESX hosts
//   - ESX_PWD: Password for ESX hosts
//   - ESX_BUILD: ESX build number (e.g., "ob-12345" or "sb-67890")
func NewESXConfigFromEnv() *ESXConfig {
	esxIPs := os.Getenv("ESX_IPS")
	esxUser := os.Getenv("ESX_USER")
	esxPwd := os.Getenv("ESX_PWD")
	esxBuild := os.Getenv("ESX_BUILD")

	if esxIPs == "" || esxUser == "" || esxPwd == "" || esxBuild == "" {
		// ESX configuration not provided
		return nil
	}

	return &ESXConfig{
		HostIPs:  ParseESXHosts(esxIPs),
		Username: esxUser,
		Password: esxPwd,
		Build:    esxBuild,
	}
}

// kubeCreateAlreadyExists returns true if kubectl "create" failed only because the object already exists.
func kubeCreateAlreadyExists(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "AlreadyExists") || strings.Contains(s, "already exists")
}

// SetupClusterRoleBindings creates the necessary cluster role bindings for vm-operator e2e tests.
// This replaces the Python logic from roles_helper.py that was called by gce2e_prerequisite.py.
// It SSHes into the supervisor control plane as root to run kubectl with admin.conf, exactly as
// the Python script did with run_cmd_vc on the supervisor control plane IP.
func SetupClusterRoleBindings(clusterProxy *common.VMServiceClusterProxy) error {
	ctx := context.TODO()

	e2eframework.Byf("Setting up cluster role bindings for e2e tests")

	// Get an admin cluster proxy that uses /etc/kubernetes/admin.conf from the control plane VM,
	// giving the cluster-admin privileges.
	adminProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get admin cluster proxy: %w", err)
	}
	defer adminProxy.Dispose(ctx)

	// Define the custom roles needed for e2e testing (from roles_helper.py)
	customRoles := []RBACRole{
		{
			Name:     "gce2e-crds",
			APIGroup: "apiextensions.k8s.io",
			Resource: "customresourcedefinitions",
			Verbs:    "get,list,watch",
		},
		{
			Name:     "gce2e-cns-batch-attachments",
			APIGroup: "cns.vmware.com",
			Resource: "cnsnodevmbatchattachments",
			Verbs:    "get,list,watch",
		},
		{
			Name:     "gce2e-import-operations",
			APIGroup: "mobility-operator.vmware.com",
			Resource: "importoperations",
			Verbs:    "create,update,patch,delete,get,list,watch",
		},
	}

	kubeconfigPath := adminProxy.GetKubeconfigPath()

	// 1. Create cluster-admin binding for cluster-administrator@vsphere.local (best effort)
	clusterAdminArgs := []string{
		"--kubeconfig", kubeconfigPath,
		"--insecure-skip-tls-verify",
		"create", "clusterrolebinding", "cluster-administrator:cluster-admin",
		"--user", "sso:cluster-administrator@vsphere.local",
		"--clusterrole", "cluster-admin",
	}
	clusterAdminCmd := e2eframework.NewCommand(
		e2eframework.WithCommand("kubectl"),
		e2eframework.WithArgs(clusterAdminArgs...),
	)

	_, _, err = clusterAdminCmd.Run(ctx)
	if err != nil {
		// This is best effort - may already exist
		framework.Logf("Info: cluster-admin binding may already exist: %v", err)
	}

	// 2. Create custom cluster roles and bindings for Administrator@vsphere.local
	for _, role := range customRoles {
		// Create the cluster role
		createRoleArgs := []string{
			"--kubeconfig", kubeconfigPath,
			"--insecure-skip-tls-verify",
			"create", "clusterrole", role.Name,
			"--verb", role.Verbs,
			"--resource", role.Resource + "." + role.APIGroup,
		}
		createRoleCmd := e2eframework.NewCommand(
			e2eframework.WithCommand("kubectl"),
			e2eframework.WithArgs(createRoleArgs...),
		)

		_, stderr, err := createRoleCmd.Run(ctx)
		if err != nil {
			if kubeCreateAlreadyExists(stderr) {
				framework.Logf("Info: cluster role %q already exists, skipping create", role.Name)
			} else {
				return fmt.Errorf("failed to create cluster role %s: %w\nstderr: %s", role.Name, err, string(stderr))
			}
		}

		// Create the cluster role binding
		createBindingArgs := []string{
			"--kubeconfig", kubeconfigPath,
			"--insecure-skip-tls-verify",
			"create", "clusterrolebinding", role.Name,
			"--user", "sso:Administrator@vsphere.local",
			"--clusterrole", role.Name,
		}
		createBindingCmd := e2eframework.NewCommand(
			e2eframework.WithCommand("kubectl"),
			e2eframework.WithArgs(createBindingArgs...),
		)

		_, stderr, err = createBindingCmd.Run(ctx)
		if err != nil {
			if kubeCreateAlreadyExists(stderr) {
				framework.Logf("Info: cluster role binding %q already exists, skipping create", role.Name)
			} else {
				return fmt.Errorf("failed to create cluster role binding %s: %w\nstderr: %s", role.Name, err, string(stderr))
			}
		}
	}

	framework.Logf("Successfully set up cluster role bindings for e2e tests")

	return nil
}

// CleanupClusterRoleBindings removes the cluster role bindings created by SetupClusterRoleBindings.
// This can be called in test cleanup to avoid leaving test artifacts behind.
func CleanupClusterRoleBindings(clusterProxy *common.VMServiceClusterProxy) error {
	ctx := context.TODO()

	e2eframework.Byf("Cleaning up cluster role bindings for e2e tests")

	// Use admin proxy with /etc/kubernetes/admin.conf for elevated privileges.
	adminProxy, err := clusterProxy.NewAdminClusterProxy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get admin cluster proxy: %w", err)
	}
	defer adminProxy.Dispose(ctx)

	// Define the same custom roles for cleanup
	customRoles := []RBACRole{
		{Name: "gce2e-crds"},
		{Name: "gce2e-cns-batch-attachments"},
		{Name: "gce2e-import-operations"},
	}

	kubeconfigPath := adminProxy.GetKubeconfigPath()

	// Remove cluster-admin binding (best effort)
	deleteClusterAdminArgs := []string{
		"--kubeconfig", kubeconfigPath,
		"--insecure-skip-tls-verify",
		"delete", "clusterrolebinding", "cluster-administrator:cluster-admin",
	}
	deleteClusterAdminCmd := e2eframework.NewCommand(
		e2eframework.WithCommand("kubectl"),
		e2eframework.WithArgs(deleteClusterAdminArgs...),
	)

	_, _, err = deleteClusterAdminCmd.Run(ctx)
	if err != nil {
		framework.Logf("Warning: Failed to delete cluster-admin binding: %v", err)
	}

	// Remove custom cluster roles and bindings
	for _, role := range customRoles {
		// Delete the cluster role binding
		deleteBindingArgs := []string{
			"--kubeconfig", kubeconfigPath,
			"--insecure-skip-tls-verify",
			"delete", "clusterrolebinding", role.Name,
		}
		deleteBindingCmd := e2eframework.NewCommand(
			e2eframework.WithCommand("kubectl"),
			e2eframework.WithArgs(deleteBindingArgs...),
		)

		_, _, err := deleteBindingCmd.Run(ctx)
		if err != nil {
			framework.Logf("Warning: Failed to delete cluster role binding %s: %v", role.Name, err)
		}

		// Delete the cluster role
		deleteRoleArgs := []string{
			"--kubeconfig", kubeconfigPath,
			"--insecure-skip-tls-verify",
			"delete", "clusterrole", role.Name,
		}
		deleteRoleCmd := e2eframework.NewCommand(
			e2eframework.WithCommand("kubectl"),
			e2eframework.WithArgs(deleteRoleArgs...),
		)

		_, _, err = deleteRoleCmd.Run(ctx)
		if err != nil {
			framework.Logf("Warning: Failed to delete cluster role %s: %v", role.Name, err)
		}
	}

	framework.Logf("Successfully cleaned up cluster role bindings for e2e tests")

	return nil
}
