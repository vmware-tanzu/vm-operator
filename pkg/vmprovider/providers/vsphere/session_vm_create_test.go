// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package vsphere

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

var _ = Describe("deploy VM", func() {
	Context("getClusterVMConfigOptions", func() {
		It("passes with simulator's default cluster compute resource", func() {
			res := simulator.VPX().Run(func(ctx context.Context, c *vim25.Client) error {
				finder := find.NewFinder(c)
				cluster, err := finder.DefaultClusterComputeResource(ctx)
				Expect(err).ToNot(HaveOccurred())
				ids, hwVersion, err := getClusterVMConfigOptions(ctx, cluster, c)
				Expect(err).To(BeNil())
				Expect(ids).ToNot(BeNil())
				Expect(hwVersion).ToNot(BeNil())
				return nil
			})
			Expect(res).To(BeNil())
		})
	})

	Context("preCheck", func() {
		var (
			vmCtx              VMCloneContext
			vmConfig           vmprovider.VmConfigArgs
			vmImage            *vmopv1alpha1.VirtualMachineImage
			clusterHwVersion   int32
			guestOSIdsToFamily map[string]string
			dummyValidOsType   = "dummy_valid_os_type"
			dummyEmptyOsType   = ""
			dummyWindowsOSType = "dummy_win_os"
			dummyLinuxFamily   = string(types.VirtualMachineGuestOsFamilyLinuxGuest)
			dummyWindowsFamily = string(types.VirtualMachineGuestOsFamilyWindowsGuest)
		)

		BeforeEach(func() {
			vmCtx.VM = &vmopv1alpha1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-vm",
				},
			}

			vmImage = &vmopv1alpha1.VirtualMachineImage{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-image",
				},
				Spec: vmopv1alpha1.VirtualMachineImageSpec{
					OSInfo: vmopv1alpha1.VirtualMachineImageOSInfo{
						Type: dummyValidOsType,
					},
				},
			}
			guestOSIdsToFamily = make(map[string]string)
			guestOSIdsToFamily[dummyValidOsType] = dummyLinuxFamily
			guestOSIdsToFamily[dummyWindowsOSType] = dummyWindowsFamily
		})
		It("passes when osType is Linux", func() {
			vmConfig.VmImage = vmImage
			Expect(checkVMConfigOptions(vmCtx, vmConfig, clusterHwVersion, guestOSIdsToFamily)).To(Succeed())
		})
		It("fails when osType is not Linux", func() {
			vmImage.Spec.OSInfo.Type = dummyWindowsOSType
			vmConfig.VmImage = vmImage
			err := checkVMConfigOptions(vmCtx, vmConfig, clusterHwVersion, guestOSIdsToFamily)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("image osType '%s' is not "+
				"supported by VMService", dummyWindowsOSType)))
		})
		It("fails when osType is empty", func() {
			vmImage.Spec.OSInfo.Type = dummyEmptyOsType
			vmConfig.VmImage = vmImage
			err := checkVMConfigOptions(vmCtx, vmConfig, clusterHwVersion, guestOSIdsToFamily)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("image osType '%s' is not "+
				"supported by VMService", dummyEmptyOsType)))
		})
		It("passes when osType is invalid and VMOperatorImageSupportedCheckKey==disable annotation is set", func() {
			vmImage.Spec.OSInfo.Type = dummyWindowsOSType
			vmConfig.VmImage = vmImage
			vmCtx.VM.Annotations = make(map[string]string)
			vmCtx.VM.Annotations[VMOperatorImageSupportedCheckKey] = VMOperatorImageSupportedCheckDisable
			Expect(checkVMConfigOptions(vmCtx, vmConfig, clusterHwVersion, guestOSIdsToFamily)).To(Succeed())
		})
		It("fails when hardware version for thc cluster is lower than the image", func() {
			vmImage.Spec.HardwareVersion = 12
			vmConfig.VmImage = vmImage
			err := checkVMConfigOptions(vmCtx, vmConfig, 10, guestOSIdsToFamily)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(fmt.Sprintf("image has a hardware version '%d' higher than "+
				"cluster's default hardware version '%d'", 12, 10)))
		})

		It("passes when hardware version for thc cluster higher than the image", func() {
			vmImage.Spec.HardwareVersion = 12
			vmConfig.VmImage = vmImage
			Expect(checkVMConfigOptions(vmCtx, vmConfig, 14, guestOSIdsToFamily)).To(Succeed())
		})
	})
})
