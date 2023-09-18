// Copyright (c) 2021-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"github.com/vmware-tanzu/vm-operator/pkg"
)

const (
	ExtraConfigTrue            = "TRUE"
	ExtraConfigFalse           = "FALSE"
	ExtraConfigUnset           = ""
	ExtraConfigGuestInfoPrefix = "guestinfo."

	// VCVMAnnotation Annotation placed on the VM.
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// ManagedByExtensionKey and ManagedByExtensionType represent the ManagedBy field on the VM.
	// Historically, this field was used to differentiate VM Service managed VMs from traditional ones.
	ManagedByExtensionKey  = "com.vmware.vcenter.wcp"
	ManagedByExtensionType = "VirtualMachine"

	// VSphereCustomizationBypassKey Annotation to skip applying VMware Tools Guest Customization.
	VSphereCustomizationBypassKey     = pkg.VMOperatorKey + "/vsphere-customization"
	VSphereCustomizationBypassDisable = "disable"

	// VMOperatorV1Alpha1ExtraConfigKey Special ExtraConfig key for v1alpha1 images.
	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"

	// GOSCPendingExtraConfigKey and GOSCIgnoreToolsCheckExtraConfigKey are GOSC Related ExtraConfig keys.
	GOSCPendingExtraConfigKey          = "tools.deployPkg.fileName"
	GOSCIgnoreToolsCheckExtraConfigKey = "vmware.tools.gosc.ignoretoolscheck"

	// EnableDiskUUIDExtraConfigKey Enable UUID ExtraConfig key.
	EnableDiskUUIDExtraConfigKey = "disk.enableUUID"

	// MMPowerOffVMExtraConfigKey ExtraConfig key to enable DRS to powerOff VMs when underlying host enters into
	// maintenance mode. This is to ensure the maintenance mode workflow is consistent for VMs with vGPU/DDPIO devices.
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"

	// NetPlanVersion points to the version used for Network config.
	// For more information, please see https://cloudinit.readthedocs.io/en/latest/topics/network-config-format-v2.html
	NetPlanVersion = 2

	// VMImageCLVersionAnnotation VirtualMachineImage annotation to cache the last fetched version.
	VMImageCLVersionAnnotation = pkg.VMOperatorKey + "/content-library-version"
	// VMImageCLVersionAnnotationVersion is the version of the VMImageCLVersionAnnotation for the VirtualMachineImage.
	VMImageCLVersionAnnotationVersion = 1

	PCIPassthruMMIOOverrideAnnotation = pkg.VMOperatorKey + "/pci-passthru-64bit-mmio-size"
	PCIPassthruMMIOExtraConfigKey     = "pciPassthru.use64bitMMIO"    //nolint:gosec
	PCIPassthruMMIOSizeExtraConfigKey = "pciPassthru.64bitMMIOSizeGB" //nolint:gosec
	PCIPassthruMMIOSizeDefault        = "512"

	// MinSupportedHWVersionForPVC is the supported virtual hardware version for persistent volumes.
	MinSupportedHWVersionForPVC = 15
	// MinSupportedHWVersionForPCIPassthruDevices is the supported virtual hardware version for NVidia PCI devices.
	MinSupportedHWVersionForPCIPassthruDevices = 17

	// FirmwareOverrideAnnotation is the annotation key used for firmware override.
	FirmwareOverrideAnnotation = pkg.VMOperatorKey + "/firmware"

	CloudInitTypeAnnotation         = pkg.VMOperatorKey + "/cloudinit-type"
	CloudInitTypeValueCloudInitPrep = "cloudinitprep"
	CloudInitTypeValueGuestInfo     = "guestinfo"

	CloudInitGuestInfoMetadata         = "guestinfo.metadata"
	CloudInitGuestInfoMetadataEncoding = "guestinfo.metadata.encoding"
	CloudInitGuestInfoUserdata         = "guestinfo.userdata"
	CloudInitGuestInfoUserdataEncoding = "guestinfo.userdata.encoding"

	// InstanceStoragePVCNamePrefix prefix of auto-generated PVC names.
	InstanceStoragePVCNamePrefix = "instance-pvc-"
	// InstanceStorageLabelKey identifies resources related to instance storage.
	// The primary purpose of this label is to identify instance storage resources such as
	// PVCs and CNSNodeVMAttachments but not for List and kubectl-get of VM resources.
	InstanceStorageLabelKey = "vmoperator.vmware.com/instance-storage-resource"
	// InstanceStoragePVCsBoundAnnotationKey annotation key used to set bound state of all instance storage PVCs.
	InstanceStoragePVCsBoundAnnotationKey = "vmoperator.vmware.com/instance-storage-pvcs-bound"
	// InstanceStoragePVPlacementErrorAnnotationKey annotation key to set PV creation error.
	// CSI reference to this annotation where it is defined:
	// https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/master/pkg/syncer/k8scloudoperator/placement.go
	InstanceStoragePVPlacementErrorAnnotationKey = "failure-domain.beta.vmware.com/storagepool"
	// InstanceStorageSelectedNodeMOIDAnnotationKey value corresponds to MOID of ESXi node that is elected to place instance storage volumes.
	InstanceStorageSelectedNodeMOIDAnnotationKey = "vmoperator.vmware.com/instance-storage-selected-node-moid"
	// InstanceStorageSelectedNodeAnnotationKey value corresponds to FQDN of ESXi node that is elected to place instance storage volumes.
	InstanceStorageSelectedNodeAnnotationKey = "vmoperator.vmware.com/instance-storage-selected-node"
	// KubernetesSelectedNodeAnnotationKey annotation key to set selected node on PVC.
	KubernetesSelectedNodeAnnotationKey = "volume.kubernetes.io/selected-node"
	// InstanceStoragePVPlacementErrorPrefix indicates prefix of error value.
	InstanceStoragePVPlacementErrorPrefix = "FAILED_"
	// InstanceStorageNotEnoughResErr is an error constant to indicate not enough resources.
	InstanceStorageNotEnoughResErr = "FAILED_PLACEMENT-NotEnoughResources"
	// InstanceStorageVDiskID vDisk ID for instance storage volume.
	InstanceStorageVDiskID = "cc737f33-2aa3-4594-aa60-df7d6d4cb984"

	// XsiNamespace indicates the XML scheme instance namespace.
	XsiNamespace = "http://www.w3.org/2001/XMLSchema-instance"
	// ConfigSpecProviderXML indicates XML as the config spec transport type for virtual machine deployment.
	ConfigSpecProviderXML = "XML"

	// V1alpha1FirstIP is an alias for versioned templating function V1alpha1_FirstIP.
	V1alpha1FirstIP = "V1alpha1_FirstIP"
	// V1alpha1FirstNicMacAddr is an alias for versioned templating function V1alpha1_FirstNicMacAddr.
	V1alpha1FirstNicMacAddr = "V1alpha1_FirstNicMacAddr"
	// V1alpha1FirstIPFromNIC is an alias for versioned templating function V1alpha1_FirstIPFromNIC.
	V1alpha1FirstIPFromNIC = "V1alpha1_FirstIPFromNIC"
	// V1alpha1IPsFromNIC is an alias for versioned templating function V1alpha1_IPsFromNIC.
	V1alpha1IPsFromNIC = "V1alpha1_IPsFromNIC"
	// V1alpha1FormatIP is an alias for versioned templating function V1alpha1_FormatIP.
	V1alpha1FormatIP = "V1alpha1_FormatIP"
	// V1alpha1IP is an alias for versioned templating function V1alpha1_IP.
	V1alpha1IP = "V1alpha1_IP"
	// V1alpha1SubnetMask is an alias for versioned templating function  V1alpha1_SubnetMask.
	V1alpha1SubnetMask = "V1alpha1_SubnetMask"
	// V1alpha1FormatNameservers is an alias for versioned templating function V1alpha1_FormatNameservers.
	V1alpha1FormatNameservers = "V1alpha1_FormatNameservers"

	// BackupVMKubeDataExtraConfigKey is the ExtraConfig key to the VirtualMachine
	// resource's Kubernetes spec data, compressed using gzip and base64-encoded.
	BackupVMKubeDataExtraConfigKey = "vmservice.virtualmachine.kubedata"
	// BackupVMBootstrapDataExtraConfigKey is the ExtraConfig key to the VM's
	// bootstrap data object, compressed using gzip and base64-encoded.
	BackupVMBootstrapDataExtraConfigKey = "vmservice.virtualmachine.bootstrapdata"
)
