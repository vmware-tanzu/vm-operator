// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package constants

import (
	"github.com/vmware-tanzu/vm-operator/pkg"
)

const (
	ExtraConfigTrue                    = "TRUE"
	ExtraConfigFalse                   = "FALSE"
	ExtraConfigUnset                   = ""
	ExtraConfigGuestInfoPrefix         = "guestinfo."
	ExtraConfigRunContainerKey         = "RUN.container"
	ExtraConfigVMServiceNamespacedName = "vmservice.namespacedName"
	ExtraConfigReservedProfileID       = "resourcepool.vmResourceProfileId"

	// VCVMAnnotation Annotation placed on the VM.
	VCVMAnnotation = "Virtual Machine managed by the vSphere Virtual Machine service"

	// VSphereCustomizationBypassKey Annotation to skip applying VMware Tools Guest Customization.
	VSphereCustomizationBypassKey     = pkg.VMOperatorKey + "/vsphere-customization"
	VSphereCustomizationBypassDisable = "disable"

	// VMPausedByAdminError is an error thrown during VM deletion. Because admin paused VM,
	// deletion operation is paused.
	VMPausedByAdminError = "failed to delete this VM because extraConfig Key 'vmservice.virtualmachine.pause' is set by admin"

	// VMOperatorV1Alpha1ExtraConfigKey Special ExtraConfig key for v1alpha1 images.
	VMOperatorV1Alpha1ExtraConfigKey = "guestinfo.vmservice.defer-cloud-init"
	VMOperatorV1Alpha1ConfigReady    = "ready"
	VMOperatorV1Alpha1ConfigEnabled  = "enabled"

	// GOSCPendingExtraConfigKey and GOSCIgnoreToolsCheckExtraConfigKey are GOSC Related ExtraConfig keys.
	GOSCPendingExtraConfigKey          = "tools.deployPkg.fileName"
	GOSCIgnoreToolsCheckExtraConfigKey = "vmware.tools.gosc.ignoretoolscheck"

	// EnableDiskUUIDExtraConfigKey Enable UUID ExtraConfig key.
	EnableDiskUUIDExtraConfigKey = "disk.enableUUID"

	// MMPowerOffVMExtraConfigKey ExtraConfig key to enable DRS to powerOff VMs
	// when the underlying host enters into maintenance mode. This is to ensure
	// the maintenance mode workflow is consistent for VMs with vGPU/DDPIO
	// devices.
	// Deprecated: Admins can power off the VMs as they would VMs that are not
	//             managed by VM Operator.
	MMPowerOffVMExtraConfigKey = "maintenance.vm.evacuation.poweroff"

	// NetPlanVersion points to the version used for Network config.
	// For more information, please see https://cloudinit.readthedocs.io/en/latest/topics/network-config-format-v2.html
	NetPlanVersion = int64(2)

	PCIPassthruMMIOOverrideAnnotation = pkg.VMOperatorKey + "/pci-passthru-64bit-mmio-size"
	PCIPassthruMMIOExtraConfigKey     = "pciPassthru.use64bitMMIO"    //nolint:gosec
	PCIPassthruMMIOSizeExtraConfigKey = "pciPassthru.64bitMMIOSizeGB" //nolint:gosec
	PCIPassthruMMIOSizeDefault        = "512"

	// FirmwareOverrideAnnotation is the annotation key used for firmware override.
	FirmwareOverrideAnnotation = pkg.VMOperatorKey + "/firmware"

	CloudInitTypeAnnotation         = pkg.VMOperatorKey + "/cloudinit-type"
	CloudInitTypeValueCloudInitPrep = "cloudinitprep"
	CloudInitTypeValueGuestInfo     = "guestinfo"

	CloudInitGuestInfoMetadata         = "guestinfo.metadata"
	CloudInitGuestInfoMetadataEncoding = "guestinfo.metadata.encoding"
	CloudInitGuestInfoUserdata         = "guestinfo.userdata"
	CloudInitGuestInfoUserdataEncoding = "guestinfo.userdata.encoding"

	// CloudInitGuestInfoLocalIPv4Key and CloudInitGuestInfoLocalIPv6Key are the local IPs
	// reported by the VMware datasource: https://bit.ly/3NJB534.
	CloudInitGuestInfoLocalIPv4Key = "guestinfo.local-ipv4"
	CloudInitGuestInfoLocalIPv6Key = "guestinfo.local-ipv6"

	// EncryptionClassNameAnnotation specifies the name of an EncryptionClass
	// resource. This is used by APIs that participate in BYOK but cannot modify
	// their spec to do so, such as the PersistentVolumeClaim API.
	EncryptionClassNameAnnotation = "encryption.vmware.com/encryption-class-name"

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

	// VPCAttachmentRef annotation key is for VPC SubnetPort to get virtual machine name.
	VPCAttachmentRef = "nsx.vmware.com/attachment_ref"

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
	// V1alpha2FirstIP is an alias for versioned templating function V1alpha2_FirstIP.
	V1alpha2FirstIP = "V1alpha2_FirstIP"
	// V1alpha2FirstNicMacAddr is an alias for versioned templating function V1alpha2_FirstNicMacAddr.
	V1alpha2FirstNicMacAddr = "V1alpha2_FirstNicMacAddr"
	// V1alpha2FirstIPFromNIC is an alias for versioned templating function V1alpha2_FirstIPFromNIC.
	V1alpha2FirstIPFromNIC = "V1alpha2_FirstIPFromNIC"
	// V1alpha2IPsFromNIC is an alias for versioned templating function V1alpha2_IPsFromNIC.
	V1alpha2IPsFromNIC = "V1alpha2_IPsFromNIC"
	// V1alpha2FormatIP is an alias for versioned templating function V1alpha2_FormatIP.
	V1alpha2FormatIP = "V1alpha2_FormatIP"
	// V1alpha2IP is an alias for versioned templating function V1alpha2_IP.
	V1alpha2IP = "V1alpha2_IP"
	// V1alpha2SubnetMask is an alias for versioned templating function  V1alpha2_SubnetMask.
	V1alpha2SubnetMask = "V1alpha2_SubnetMask"
	// V1alpha2FormatNameservers is an alias for versioned templating function V1alpha2_FormatNameservers.
	V1alpha2FormatNameservers = "V1alpha2_FormatNameservers"
	// V1alpha3FirstIP is an alias for versioned templating function V1alpha3_FirstIP.
	V1alpha3FirstIP = "V1alpha3_FirstIP"
	// V1alpha3FirstNicMacAddr is an alias for versioned templating function V1alpha3_FirstNicMacAddr.
	V1alpha3FirstNicMacAddr = "V1alpha3_FirstNicMacAddr"
	// V1alpha3FirstIPFromNIC is an alias for versioned templating function V1alpha3_FirstIPFromNIC.
	V1alpha3FirstIPFromNIC = "V1alpha3_FirstIPFromNIC"
	// V1alpha3IPsFromNIC is an alias for versioned templating function V1alpha3_IPsFromNIC.
	V1alpha3IPsFromNIC = "V1alpha3_IPsFromNIC"
	// V1alpha3FormatIP is an alias for versioned templating function V1alpha3_FormatIP.
	V1alpha3FormatIP = "V1alpha3_FormatIP"
	// V1alpha3IP is an alias for versioned templating function V1alpha3_IP.
	V1alpha3IP = "V1alpha3_IP"
	// V1alpha3SubnetMask is an alias for versioned templating function  V1alpha3_SubnetMask.
	V1alpha3SubnetMask = "V1alpha3_SubnetMask"
	// V1alpha3FormatNameservers is an alias for versioned templating function V1alpha3_FormatNameservers.
	V1alpha3FormatNameservers = "V1alpha3_FormatNameservers"
	// V1alpha4FirstIP is an alias for versioned templating function V1alpha4_FirstIP.
	V1alpha4FirstIP = "V1alpha4_FirstIP"
	// V1alpha4FirstNicMacAddr is an alias for versioned templating function V1alpha4_FirstNicMacAddr.
	V1alpha4FirstNicMacAddr = "V1alpha4_FirstNicMacAddr"
	// V1alpha4FirstIPFromNIC is an alias for versioned templating function V1alpha4_FirstIPFromNIC.
	V1alpha4FirstIPFromNIC = "V1alpha4_FirstIPFromNIC"
	// V1alpha4IPsFromNIC is an alias for versioned templating function V1alpha4_IPsFromNIC.
	V1alpha4IPsFromNIC = "V1alpha4_IPsFromNIC"
	// V1alpha4FormatIP is an alias for versioned templating function V1alpha4_FormatIP.
	V1alpha4FormatIP = "V1alpha4_FormatIP"
	// V1alpha4IP is an alias for versioned templating function V1alpha4_IP.
	V1alpha4IP = "V1alpha4_IP"
	// V1alpha4SubnetMask is an alias for versioned templating function  V1alpha4_SubnetMask.
	V1alpha4SubnetMask = "V1alpha4_SubnetMask"
	// V1alpha4FormatNameservers is an alias for versioned templating function V1alpha4_FormatNameservers.
	V1alpha4FormatNameservers = "V1alpha4_FormatNameservers"
)
