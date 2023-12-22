// Copyright (c) 2021-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	goctx "context"
	"encoding/base64"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha2/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vmlifecycle"
)

var _ = Describe("CloudInit Bootstrap", func() {
	const (
		cloudInitMetadata = "cloud-init-metadata"
		cloudInitUserdata = "cloud-init-userdata"
	)

	var (
		bsArgs     vmlifecycle.BootstrapArgs
		configInfo *types.VirtualMachineConfigInfo

		metaData string
		userData string
	)

	BeforeEach(func() {
		configInfo = &types.VirtualMachineConfigInfo{}
		bsArgs.Data = map[string]string{}

		// Set defaults.
		metaData = cloudInitMetadata
		userData = cloudInitUserdata
	})

	AfterEach(func() {
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	// v1a1 tests really only tested the lower level functions individually. Those tests are ported after
	// this Context, but we should focus more on testing via this just method.
	Context("BootStrapCloudInit", func() {
		var (
			configSpec *types.VirtualMachineConfigSpec
			custSpec   *types.CustomizationSpec
			err        error

			vmCtx         context.VirtualMachineContextA2
			vm            *vmopv1.VirtualMachine
			cloudInitSpec *vmopv1.VirtualMachineBootstrapCloudInitSpec
		)

		BeforeEach(func() {
			cloudInitSpec = &vmopv1.VirtualMachineBootstrapCloudInitSpec{}

			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cloud-init-bootstrap-test",
					Namespace:   "test-ns",
					UID:         "my-vm-uuid",
					Annotations: map[string]string{},
				},
			}

			vmCtx = context.VirtualMachineContextA2{
				Context: goctx.Background(),
				Logger:  suite.GetLogger(),
				VM:      vm,
			}
		})

		JustBeforeEach(func() {
			configSpec, custSpec, err = vmlifecycle.BootStrapCloudInit(
				vmCtx,
				configInfo,
				cloudInitSpec,
				&bsArgs,
			)
		})

		Context("Inlined CloudConfig", func() {
			BeforeEach(func() {
				bsArgs.CloudConfig = &cloudinit.CloudConfigSecretData{
					Users: map[string]cloudinit.CloudConfigUserSecretData{
						"bob.wilson": {
							HashPasswd: "0123456789",
						},
					},
					WriteFiles: map[string]string{
						"/hi":    "there",
						"/hello": "world",
					},
				}
				cloudInitSpec.CloudConfig = &vmopv1cloudinit.CloudConfig{
					Users: []vmopv1cloudinit.User{
						{
							Name: "bob.wilson",
							HashedPasswd: &common.SecretKeySelector{
								Name: "my-bootstrap-data",
								Key:  "cloud-init-user-bob.wilson-hashed_passwd",
							},
						},
					},
					WriteFiles: []vmopv1cloudinit.WriteFile{
						{
							Path:    "/hello",
							Content: []byte(`"world"`),
						},
						{
							Path:    "/hi",
							Content: []byte(`{"name":"my-bootstrap-data","key":"cloud-init-files-hi"}`),
						},
					},
				}
			})

			Context("With no default user", func() {
				It("Should return valid data", func() {
					Expect(custSpec).To(BeNil())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
					Expect(*configSpec.VAppConfigRemoved).To(BeTrue())

					extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata))
					Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
					act, err := util.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
					Expect(err).ToNot(HaveOccurred())
					exp, err := util.TryToDecodeBase64Gzip([]byte("H4sIALlxe2UAA1XMSw7DIAxF0blXYYVx+v+ymYgEpxA5gMAR2y+qOsn43XeUQqE1sRHSuPiwGFATx832Uwyz/wBshXLRgNijM8WRHZIppVqN3el8ud7uj+fr3bUdMZi1KWMcD9VziQFq9kLD7Jn+QkOFgmisMbP9nZIRp/HoiDnuG3GUadd4+AJhz3mVsAAAAA=="))
					Expect(err).ToNot(HaveOccurred())
					Expect(act).To(Equal(exp))
					Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
				})
			})

			Context("With default user", func() {
				BeforeEach(func() {
					cloudInitSpec.CloudConfig.DefaultUserEnabled = true
				})
				It("Should return valid data", func() {
					Expect(custSpec).To(BeNil())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
					Expect(*configSpec.VAppConfigRemoved).To(BeTrue())

					extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata))
					Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
					act, err := util.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
					Expect(err).ToNot(HaveOccurred())
					exp, err := util.TryToDecodeBase64Gzip([]byte("H4sIANFxe2UAA1WNSw6EIBBE95yio2vn/3G4jGmlHTAtEGjD9YeY2birpF69alsQWiOjkIbF+QVVO3HYTDcFP7uvUlumlLUC6MDQjBvLni1mS2aImHMxGprL9XZ/PF/v/tPUHsDjWo1jGE/FcQ5eleSEhtkx/W31QMiLhhISm30UUayGsyXmcGTEUqID49QPnrBjF7wAAAA="))
					Expect(err).ToNot(HaveOccurred())
					Expect(act).To(Equal(exp))
					Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
				})
			})

			Context("With runcmds", func() {
				BeforeEach(func() {
					cloudInitSpec.CloudConfig.RunCmd = []byte(`["ls /",["ls","-a","-l","/"],["echo","hello, world."]]`)
				})
				It("Should return valid data", func() {
					Expect(custSpec).To(BeNil())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
					Expect(*configSpec.VAppConfigRemoved).To(BeTrue())

					extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata))
					Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
					act, err := util.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
					Expect(err).ToNot(HaveOccurred())
					exp, err := util.TryToDecodeBase64Gzip([]byte("H4sIAPZxe2UAA1WOSw6EIBBE95yio9tR5//hMgalHTAtGMBw/WFQF+5eql91qiwh4DSTCMhh1GYUrOzJLrLqrRn0l7HFo/OcAVSghFco21l4HyWH4ny53u6P5+v9KdIdwIgpfelsV0dN3hrmFtNPci2ThybDH7OeUOxAG+wK9spukUIie4JoHcmaRacDtoMm3EalnQFN4KuQO7MIikOTi0cnKHR4cDT7AUt79Z4DAQAA"))
					Expect(err).ToNot(HaveOccurred())
					Expect(act).To(Equal(exp))
					Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
				})
			})
		})

		Context("RawCloudConfig", func() {
			BeforeEach(func() {
				cloudInitSpec.RawCloudConfig = &common.SecretKeySelector{}
				cloudInitSpec.RawCloudConfig.Name = "my-data"
				cloudInitSpec.RawCloudConfig.Key = "my-key"
				bsArgs.Data[cloudInitSpec.RawCloudConfig.Key] = cloudInitUserdata
			})

			Context("Via CloudInitPrep", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations[constants.CloudInitTypeAnnotation] = constants.CloudInitTypeValueCloudInitPrep
				})

				It("Returns success", func() {
					Expect(err).ToNot(HaveOccurred())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfig).ToNot(BeNil())
					Expect(configSpec.VAppConfig.GetVmConfigSpec()).ToNot(BeNil())
					Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport).To(HaveLen(1))
					Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport[0]).To(Equal(vmlifecycle.OvfEnvironmentTransportGuestInfo))

					Expect(custSpec).ToNot(BeNil())
					cloudInitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
					Expect(cloudInitPrepSpec.Metadata).ToNot(BeEmpty()) // TODO: Better assertion (reduce w/ GetCloudInitMetadata)
					Expect(cloudInitPrepSpec.Userdata).To(Equal(cloudInitUserdata))
				})
			})

			Context("Via GuestInfo", func() {
				BeforeEach(func() {
					vmCtx.VM.Annotations[constants.CloudInitTypeAnnotation] = constants.CloudInitTypeValueGuestInfo
				})

				It("Returns Success", func() {
					Expect(err).ToNot(HaveOccurred())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
					Expect(*configSpec.VAppConfigRemoved).To(BeTrue())

					extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata)) // TODO: Better assertion (reduce w/ GetCloudInitMetadata)
					Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
					Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
					Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))

					Expect(custSpec).To(BeNil())
				})

				Context("Via CAPBK userdata in 'value' key", func() {
					const otherUserData = cloudInitUserdata + "CAPBK"

					BeforeEach(func() {
						bsArgs.Data[cloudInitSpec.RawCloudConfig.Key] = ""
						cloudInitSpec.RawCloudConfig.Key = ""
						bsArgs.Data["value"] = otherUserData
					})

					It("Returns success", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(configSpec).ToNot(BeNil())

						extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
						Expect(extraConfig).To(HaveLen(4))
						Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata)) // TODO: Better assertion (reduce w/ GetCloudInitMetadata)
						Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))

						Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoUserdata))
						Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))

						data, err := util.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal(otherUserData))
					})
				})
			})
		})
	})

	Context("GetCloudInitMetadata", func() {
		var (
			uid           string
			hostName      string
			netPlan       *network.Netplan
			sshPublicKeys string

			mdYaml string
			err    error
		)

		BeforeEach(func() {
			uid = "my-uid"
			hostName = "my-hostname"
			netPlan = &network.Netplan{
				Version: 42,
				Ethernets: map[string]network.NetplanEthernet{
					"eth0": {
						SetName: "eth0",
					},
				},
			}
			sshPublicKeys = "my-ssh-key"
		})

		JustBeforeEach(func() {
			mdYaml, err = vmlifecycle.GetCloudInitMetadata(uid, hostName, netPlan, sshPublicKeys)
		})

		It("DoIt", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(mdYaml).ToNot(BeEmpty())

			ciMetadata := &vmlifecycle.CloudInitMetadata{}
			Expect(yaml.Unmarshal([]byte(mdYaml), ciMetadata)).To(Succeed())

			Expect(ciMetadata.InstanceID).To(Equal(uid))
			Expect(ciMetadata.Hostname).To(Equal(hostName))
			Expect(ciMetadata.PublicKeys).To(Equal(sshPublicKeys))
			Expect(ciMetadata.Network.Version).To(Equal(42))
			Expect(ciMetadata.Network.Ethernets).To(HaveKey("eth0"))
		})
	})

	Context("GetCloudInitGuestInfoCustSpec", func() {
		var (
			configSpec *types.VirtualMachineConfigSpec
			err        error
		)

		JustBeforeEach(func() {
			configSpec, err = vmlifecycle.GetCloudInitGuestInfoCustSpec(configInfo, metaData, userData)
		})

		Context("VAppConfig Disabled", func() {
			It("Should disable the VAppConfig", func() {
				Expect(err).ToNot(HaveOccurred())

				Expect(configSpec).ToNot(BeNil())
				Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
				Expect(*configSpec.VAppConfigRemoved).To(BeTrue())
			})
		})

		Context("No userdata", func() {
			BeforeEach(func() {
				userData = ""
			})

			It("ConfigSpec.ExtraConfig to only have metadata", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())

				extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(2))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With userdata", func() {
			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				userData = base64.StdEncoding.EncodeToString([]byte(cloudInitUserdata))
			})

			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With gzipped, base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				data, err := util.EncodeGzipBase64(cloudInitUserdata)
				Expect(err).ToNot(HaveOccurred())
				userData = data
			})

			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := util.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
			})
		})

	})

	Context("GetCloudInitPrepCustSpec", func() {
		var (
			custSpec *types.CustomizationSpec
		)

		JustBeforeEach(func() {
			var configSpec *types.VirtualMachineConfigSpec
			var err error

			configSpec, custSpec, err = vmlifecycle.GetCloudInitPrepCustSpec(metaData, userData)
			Expect(err).ToNot(HaveOccurred())

			// Validate that Cloud-Init Prep always uses the GuestInfo transport for the CI Prep's meta and user data.
			Expect(configSpec).ToNot(BeNil())
			Expect(configSpec.VAppConfig).ToNot(BeNil())
			Expect(configSpec.VAppConfig.GetVmConfigSpec()).ToNot(BeNil())
			Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport).To(HaveLen(1))
			Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport[0]).To(Equal(vmlifecycle.OvfEnvironmentTransportGuestInfo))
		})

		Context("No userdata", func() {
			BeforeEach(func() {
				userData = ""
			})

			It("Cust spec to only have metadata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudInitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudInitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudInitPrepSpec.Userdata).To(BeEmpty())
			})
		})

		Context("With userdata", func() {
			It("Cust spec to have metadata and userdata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudInitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudInitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudInitPrepSpec.Userdata).To(Equal(cloudInitUserdata))
			})
		})

		Context("With base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				userData = base64.StdEncoding.EncodeToString([]byte(cloudInitUserdata))
			})

			It("Cust spec to have metadata and userdata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudInitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudInitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudInitPrepSpec.Userdata).To(Equal(cloudInitUserdata))
			})
		})

		Context("With gzipped, base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				data, err := util.EncodeGzipBase64(cloudInitUserdata)
				Expect(err).ToNot(HaveOccurred())
				userData = data
			})

			It("Cust spec to have metadata and userdata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudInitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudInitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudInitPrepSpec.Userdata).To(Equal(cloudInitUserdata))
			})
		})
	})
})
