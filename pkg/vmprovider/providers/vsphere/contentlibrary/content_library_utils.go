// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

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
		},
		Status: v1alpha1.VirtualMachineImageStatus{
			Uuid:       item.ID,
			InternalId: item.Name,
		},
	}

	if item.Type == library.ItemTypeOVF {
		if ovfEnvelope.VirtualSystem != nil {
			productInfo := v1alpha1.VirtualMachineImageProductInfo{}
			osInfo := v1alpha1.VirtualMachineImageOSInfo{}

			// Use info from the first product section in the VM image, if one exists.
			if product := ovfEnvelope.VirtualSystem.Product; len(product) > 0 {
				p := product[0]
				productInfo.Vendor = p.Vendor
				productInfo.Product = p.Product
				productInfo.Version = p.Version
				productInfo.FullVersion = p.FullVersion
			}

			// Use operating system info from the first os section in the VM image, if one exists.
			if os := ovfEnvelope.VirtualSystem.OperatingSystem; len(os) > 0 {
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
			if virtualHwSection := ovfEnvelope.VirtualSystem.VirtualHardware; len(virtualHwSection) > 0 {
				hw := virtualHwSection[0]
				if hw.System != nil && hw.System.VirtualSystemType != nil {
					hwVersion = ParseVirtualHardwareVersion(hw.System.VirtualSystemType)
				}
			}

			ovfSystemProps := GetVmwareSystemPropertiesFromOvf(ovfEnvelope)

			for k, v := range ovfSystemProps {
				image.Annotations[k] = v
			}
			image.Spec.ProductInfo = productInfo
			image.Spec.OSInfo = osInfo
			image.Spec.OVFEnv = GetUserConfigurablePropertiesFromOvf(ovfEnvelope)
			image.Spec.HardwareVersion = hwVersion

			// Allow OVF compatibility if
			// - The OVF contains the VMOperatorV1Alpha1ConfigKey key that denotes cloud-init being disabled at first-boot
			// - If it is a TKG image
			if isOVFV1Alpha1Compatible(ovfEnvelope) || isATKGImage(ovfSystemProps) {
				conditions.MarkTrue(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition)
			} else {
				msg := "VirtualMachineImage is either not a TKG image or is not compatible with VMService v1alpha1"
				conditions.MarkFalse(image, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition,
					v1alpha1.VirtualMachineImageV1Alpha1NotCompatibleReason, v1alpha1.ConditionSeverityError, msg)
			}
		}

		// Set Status.ImageSupported to combined compatibility of OVF compatibility.
		image.Status.ImageSupported = pointer.BoolPtr(conditions.IsTrue(image,
			v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition))
	}

	return image
}

// ParseVirtualHardwareVersion parses the virtual hardware version
// For eg. "vmx-15" returns 15.
func ParseVirtualHardwareVersion(vmxVersion *string) int32 {
	patternStr := `vmx-(\d+)`
	re, err := regexp.Compile(patternStr)
	if err != nil {
		return 0
	}
	// obj matches the full string and the submatch (\d+)
	// and return a []string with values
	obj := re.FindStringSubmatch(*vmxVersion)
	if len(obj) != 2 {
		return 0
	}

	version, err := strconv.ParseInt(obj[1], 10, 32)
	if err != nil {
		return 0
	}

	return int32(version)
}

func GetUserConfigurablePropertiesFromOvf(ovfEnvelope *ovf.Envelope) map[string]v1alpha1.OvfProperty {
	properties := make(map[string]v1alpha1.OvfProperty)

	if ovfEnvelope.VirtualSystem != nil {
		for _, product := range ovfEnvelope.VirtualSystem.Product {
			for _, prop := range product.Property {
				// Only show user configurable properties
				if prop.UserConfigurable != nil && *prop.UserConfigurable {
					property := v1alpha1.OvfProperty{
						Key:     prop.Key,
						Type:    prop.Type,
						Default: prop.Default,
					}
					properties[prop.Key] = property
				}
			}
		}
	}
	return properties
}

func GetVmwareSystemPropertiesFromOvf(ovfEnvelope *ovf.Envelope) map[string]string {
	properties := make(map[string]string)

	if ovfEnvelope.VirtualSystem != nil {
		for _, product := range ovfEnvelope.VirtualSystem.Product {
			for _, prop := range product.Property {
				if strings.HasPrefix(prop.Key, "vmware-system") {
					properties[prop.Key] = *prop.Default
				}
			}
		}
	}
	return properties
}

func readerFromUrl(ctx context.Context, c *rest.Client, url *url.URL) (io.ReadCloser, error) {
	p := soap.DefaultDownload
	readerStream, _, err := c.Download(ctx, url, &p)
	if err != nil {
		log.Error(err, "Error occurred when downloading file", "url", url)
		return nil, err
	}

	return readerStream, nil
}

// libItemVersionAnnotation returns the version annotation value for the item
func libItemVersionAnnotation(item *library.Item) string {
	return fmt.Sprintf("%s:%s:%d", item.ID, item.Version, constants.VMImageCLVersionAnnotationVersion)
}

// isOVFV1Alpha1Compatible checks the image if it has VMOperatorV1Alpha1ExtraConfigKey set to VMOperatorV1Alpha1ConfigReady
// in the ExtraConfig
func isOVFV1Alpha1Compatible(ovfEnvelope *ovf.Envelope) bool {
	if ovfEnvelope.VirtualSystem != nil {
		for _, virtualHardware := range ovfEnvelope.VirtualSystem.VirtualHardware {
			for _, config := range virtualHardware.ExtraConfig {
				if config.Key == constants.VMOperatorV1Alpha1ExtraConfigKey && config.Value == constants.VMOperatorV1Alpha1ConfigReady {
					return true
				}
			}
		}
	}
	return false
}

// isATKGImage validates if a VirtualMachineImage OVF is a TKG Image type
func isATKGImage(systemProperties map[string]string) bool {
	const tkgImageIdentifier = "vmware-system.guest.kubernetes"
	for key := range systemProperties {
		if strings.HasPrefix(key, tkgImageIdentifier) {
			return true
		}
	}
	return false
}
