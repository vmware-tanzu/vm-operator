// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	goctx "context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

var _ = Describe("deploy VM", func() {
	Context("getClusterVMConfigOptions", func() {
		It("passes with simulator's default cluster compute resource", func() {
			res := simulator.VPX().Run(func(ctx goctx.Context, c *vim25.Client) error {
				finder := find.NewFinder(c)
				cluster, err := finder.DefaultClusterComputeResource(ctx)
				Expect(err).ToNot(HaveOccurred())
				ids, err := session.GetClusterVMConfigOptions(ctx, cluster, c)
				Expect(err).To(BeNil())
				Expect(ids).ToNot(BeNil())
				return nil
			})
			Expect(res).To(BeNil())
		})
	})

	Context("preCheck", func() {
		var (
			vmCtx                         session.VirtualMachineCloneContext
			vmConfig                      vmprovider.VMConfigArgs
			vmImage                       *vmopv1alpha1.VirtualMachineImage
			guestOSIdsToFamily            map[string]string
			dummyValidOsType              = "dummy_valid_os_type"
			dummyEmptyOsType              = ""
			dummyWindowsOSType            = "dummy_win_os"
			dummyLinuxFamily              = string(types.VirtualMachineGuestOsFamilyLinuxGuest)
			dummyWindowsFamily            = string(types.VirtualMachineGuestOsFamilyWindowsGuest)
			oldIsUnifiedTKGBYOIFSSEnabled func() bool
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

		When("with FSS_WCP_VMSERVICE_UNIFIEDTKG_BYOI disabled", func() {
			It("passes when osType is Linux", func() {
				vmConfig.VMImage = vmImage
				Expect(session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)).To(Succeed())
			})
			It("fails when osType is not Linux", func() {
				vmImage.Spec.OSInfo.Type = dummyWindowsOSType
				vmConfig.VMImage = vmImage
				err := session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("image osType '%s' is not "+
					"supported by VMService", dummyWindowsOSType)))
			})
			It("fails when osType is empty", func() {
				vmImage.Spec.OSInfo.Type = dummyEmptyOsType
				vmConfig.VMImage = vmImage
				err := session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(fmt.Sprintf("image osType '%s' is not "+
					"supported by VMService", dummyEmptyOsType)))
			})
			It("passes when osType is invalid and VMOperatorImageSupportedCheckKey==disable annotation is set", func() {
				vmImage.Spec.OSInfo.Type = dummyWindowsOSType
				vmConfig.VMImage = vmImage
				vmCtx.VM.Annotations = make(map[string]string)
				vmCtx.VM.Annotations[constants.VMOperatorImageSupportedCheckKey] = constants.VMOperatorImageSupportedCheckDisable
				Expect(session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)).To(Succeed())
			})

		})

		When("with FSS_WCP_VMSERVICE_UNIFIEDTKG_BYOI enabled", func() {
			BeforeEach(func() {
				oldIsUnifiedTKGBYOIFSSEnabled = lib.IsUnifiedTKGBYOIFSSEnabled
				lib.IsUnifiedTKGBYOIFSSEnabled = func() bool {
					return true
				}
			})

			AfterEach(func() {
				lib.IsUnifiedTKGBYOIFSSEnabled = oldIsUnifiedTKGBYOIFSSEnabled
			})

			It("passes when osType is Linux", func() {
				vmConfig.VMImage = vmImage
				Expect(session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)).To(Succeed())
			})
			It("passes when osType is not Linux", func() {
				vmImage.Spec.OSInfo.Type = dummyWindowsOSType
				vmConfig.VMImage = vmImage
				Expect(session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)).To(Succeed())
			})
			It("passes when osType is empty", func() {
				vmImage.Spec.OSInfo.Type = dummyEmptyOsType
				vmConfig.VMImage = vmImage
				Expect(session.CheckVMConfigOptions(vmCtx, vmConfig, guestOSIdsToFamily)).To(Succeed())
			})
		})
	})
})
