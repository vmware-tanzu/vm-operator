// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/ovf"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/soap"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

var vmxRe = regexp.MustCompile(`vmx-(\d+)`)

// ParseVirtualHardwareVersion parses the virtual hardware version
// For eg. "vmx-15" returns 15.
func ParseVirtualHardwareVersion(vmxVersion string) int32 {
	// obj matches the full string and the submatch (\d+)
	// and return a []string with values
	obj := vmxRe.FindStringSubmatch(vmxVersion)
	if len(obj) != 2 {
		return 0
	}

	version, err := strconv.ParseInt(obj[1], 10, 32)
	if err != nil {
		return 0
	}

	return int32(version)
}

// LibItemToVirtualMachineImage converts a given library item and its attributes to return a
// VirtualMachineImage that represents a k8s-native view of the item.
func LibItemToVirtualMachineImage(
	item *library.Item,
	ovfEnvelope *ovf.Envelope) *v1alpha1.VirtualMachineImage {

	var ts metav1.Time
	if item.CreationTime != nil {
		ts = metav1.NewTime(*item.CreationTime)
	}

	// NOTE: Whenever a Spec/Status field, label, annotation, etc is added or removed, the or the logic
	// is changed as to what is set, the VMImageCLVersionAnnotationVersion very, very likely needs to be
	// incremented so that the VMImageCLVersionAnnotation annotations changes so the updated is actually
	// updated. This is a hack to reduce repeated ContentLibrary tasks.
	image := &v1alpha1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:              item.Name,
			CreationTimestamp: ts,
			Annotations: map[string]string{
				constants.VMImageCLVersionAnnotation: libItemVersionAnnotation(item),
			},
		},
		Spec: v1alpha1.VirtualMachineImageSpec{
			Type:            item.Type,
			ImageSourceType: "Content Library",
			ImageID:         item.ID,
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       item.ID,
			InternalId: item.Name,
			ImageName:  item.Name,
		},
	}

	if item.Type == library.ItemTypeOVF && ovfEnvelope.VirtualSystem != nil {
		updateImageSpecWithOvfVirtualSystem(&image.Spec, ovfEnvelope.VirtualSystem)

		ovfSystemProps := getVmwareSystemPropertiesFromOvf(ovfEnvelope.VirtualSystem)
		for k, v := range ovfSystemProps {
			image.Annotations[k] = v
		}

		// Set Status.ImageSupported to combined compatibility of OVF compatibility or WCP_UNIFIED_TKG FSS state.
		image.Status.ImageSupported = pointer.BoolPtr(isImageSupported(image, ovfEnvelope.VirtualSystem, ovfSystemProps))

		// Set Status Firmware from the envelope's virtual hardware section
		if virtualHwSection := ovfEnvelope.VirtualSystem.VirtualHardware; len(virtualHwSection) > 0 {
			image.Status.Firmware = getFirmwareType(virtualHwSection[0])
		}
	}

	return image
}

// UpdateVmiWithOvfEnvelope updates the given vmi object with the content of given OVF envelope.
func UpdateVmiWithOvfEnvelope(vmi client.Object, ovfEnvelope ovf.Envelope) {
	var spec *v1alpha1.VirtualMachineImageSpec
	var status *v1alpha1.VirtualMachineImageStatus
	switch vmi := vmi.(type) {
	case *v1alpha1.VirtualMachineImage:
		spec = &vmi.Spec
		status = &vmi.Status
	case *v1alpha1.ClusterVirtualMachineImage:
		spec = &vmi.Spec
		status = &vmi.Status
	}

	if ovfEnvelope.VirtualSystem != nil {
		updateImageSpecWithOvfVirtualSystem(spec, ovfEnvelope.VirtualSystem)

		ovfSystemProps := getVmwareSystemPropertiesFromOvf(ovfEnvelope.VirtualSystem)
		annotations := vmi.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
			vmi.SetAnnotations(annotations)
		}
		for k, v := range ovfSystemProps {
			annotations[k] = v
		}
		// Set Status.ImageSupported to combined compatibility of OVF compatibility or WCP_UNIFIED_TKG FSS state.
		status.ImageSupported = pointer.BoolPtr(isImageSupported(vmi, ovfEnvelope.VirtualSystem, ovfSystemProps))

		// Set Status Firmware from the envelope's virtual hardware section
		if virtualHwSection := ovfEnvelope.VirtualSystem.VirtualHardware; len(virtualHwSection) > 0 {
			status.Firmware = getFirmwareType(virtualHwSection[0])
		}
	}
}

func updateImageSpecWithOvfVirtualSystem(imageSpec *v1alpha1.VirtualMachineImageSpec, ovfVirtualSystem *ovf.VirtualSystem) {
	if ovfVirtualSystem == nil {
		return
	}

	productInfo := v1alpha1.VirtualMachineImageProductInfo{}
	osInfo := v1alpha1.VirtualMachineImageOSInfo{}

	// Use info from the first product section in the VM image, if one exists.
	if product := ovfVirtualSystem.Product; len(product) > 0 {
		p := product[0]
		productInfo.Vendor = p.Vendor
		productInfo.Product = p.Product
		productInfo.Version = p.Version
		productInfo.FullVersion = p.FullVersion
	}

	// Use operating system info from the first os section in the VM image, if one exists.
	if os := ovfVirtualSystem.OperatingSystem; len(os) > 0 {
		o := os[0]
		if o.Version != nil {
			osInfo.Version = *o.Version
		}
		if o.OSType != nil {
			osInfo.Type = *o.OSType
		}
	}

	// Use hardware section info from the VM image, if one exists.
	var hwVersion int32
	if virtualHwSection := ovfVirtualSystem.VirtualHardware; len(virtualHwSection) > 0 {
		hw := virtualHwSection[0]
		if hw.System != nil && hw.System.VirtualSystemType != nil {
			hwVersion = ParseVirtualHardwareVersion(*hw.System.VirtualSystemType)
		}
	}

	imageSpec.ProductInfo = productInfo
	imageSpec.OSInfo = osInfo
	imageSpec.OVFEnv = getUserConfigurablePropertiesFromOvf(ovfVirtualSystem)
	imageSpec.HardwareVersion = hwVersion
}

func getUserConfigurablePropertiesFromOvf(ovfVirtualSystem *ovf.VirtualSystem) map[string]v1alpha1.OvfProperty {
	properties := make(map[string]v1alpha1.OvfProperty)

	if ovfVirtualSystem != nil {
		for _, product := range ovfVirtualSystem.Product {
			for _, prop := range product.Property {
				// Only show user configurable properties
				if prop.UserConfigurable != nil && *prop.UserConfigurable {
					property := v1alpha1.OvfProperty{
						Key:         prop.Key,
						Type:        prop.Type,
						Default:     prop.Default,
						Description: pointer.StringPtrDerefOr(prop.Description, ""),
						Label:       pointer.StringPtrDerefOr(prop.Label, ""),
					}
					properties[prop.Key] = property
				}
			}
		}
	}

	return properties
}

func getVmwareSystemPropertiesFromOvf(ovfVirtualSystem *ovf.VirtualSystem) map[string]string {
	properties := make(map[string]string)

	if ovfVirtualSystem != nil {
		for _, product := range ovfVirtualSystem.Product {
			for _, prop := range product.Property {
				if strings.HasPrefix(prop.Key, "vmware-system") {
					if prop.Default != nil {
						properties[prop.Key] = *prop.Default
					}
				}
			}
		}
	}

	return properties
}

func readerFromURL(ctx context.Context, c *rest.Client, url *url.URL) (io.ReadCloser, error) {
	p := soap.DefaultDownload
	readerStream, _, err := c.Download(ctx, url, &p)
	if err != nil {
		// Log message used by VMC LINT. Refer to before making changes
		log.Error(err, "Error occurred when downloading file", "url", url)
		return nil, err
	}

	return readerStream, nil
}

// libItemVersionAnnotation returns the version annotation value for the item.
func libItemVersionAnnotation(item *library.Item) string {
	return fmt.Sprintf("%s:%s:%d", item.ID, item.Version, constants.VMImageCLVersionAnnotationVersion)
}

// getFirmwareType returns the firmware type (eg: "efi", "bios") present in the virtual hardware section of the OVF.
func getFirmwareType(hardware ovf.VirtualHardwareSection) string {
	for _, cfg := range hardware.Config {
		if cfg.Key == "firmware" {
			return cfg.Value
		}
	}
	return ""
}

type ImageConditionWrapper interface {
	conditions.Setter
	conditions.Getter
}

// isImageSupported returns true IFF:
//
// - the image is marked as compatible in the OVF or is a TKG node image
// - the WCP_UNIFIED_TKG FSS is enabled
//
// Otherwise, the image is marked as unsupported.
func isImageSupported(image client.Object, ovfVirtualSystem *ovf.VirtualSystem, ovfSystemProps map[string]string) bool {
	var genericImage ImageConditionWrapper
	switch image := image.(type) {
	case *v1alpha1.ClusterVirtualMachineImage:
		genericImage = image
	case *v1alpha1.VirtualMachineImage:
		genericImage = image
	}

	switch {
	case isOVFV1Alpha1Compatible(ovfVirtualSystem) || isATKGImage(ovfSystemProps):
		conditions.MarkTrue(genericImage, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
	case lib.IsUnifiedTKGFSSEnabled():
		return true
	default:
		conditions.MarkFalse(
			genericImage,
			v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
			v1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason,
			v1alpha1.ConditionSeverityError,
			"VirtualMachineImage is either not a TKG image or is not compatible with VMService v1alpha1",
		)
	}
	return conditions.IsTrue(genericImage, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
}

// isOVFV1Alpha1Compatible checks the image if it has VMOperatorV1Alpha1ExtraConfigKey set to VMOperatorV1Alpha1ConfigReady
// in the ExtraConfig.
func isOVFV1Alpha1Compatible(ovfVirtualSystem *ovf.VirtualSystem) bool {
	if ovfVirtualSystem != nil {
		for _, virtualHardware := range ovfVirtualSystem.VirtualHardware {
			for _, config := range virtualHardware.ExtraConfig {
				if config.Key == constants.VMOperatorV1Alpha1ExtraConfigKey && config.Value == constants.VMOperatorV1Alpha1ConfigReady {
					return true
				}
			}
		}
	}
	return false
}

// isATKGImage validates if a VirtualMachineImage OVF is a TKG Image type.
func isATKGImage(systemProperties map[string]string) bool {
	const tkgImageIdentifier = "vmware-system.guest.kubernetes"
	for key := range systemProperties {
		if strings.HasPrefix(key, tkgImageIdentifier) {
			return true
		}
	}
	return false
}
