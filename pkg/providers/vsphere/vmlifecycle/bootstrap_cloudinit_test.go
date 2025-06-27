// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"
	"encoding/base64"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1cloudinit "github.com/vmware-tanzu/vm-operator/api/v1alpha4/cloudinit"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/cloudinit"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("CloudInit Bootstrap", func() {
	const (
		cloudInitMetadata = "cloud-init-metadata"
		cloudInitUserdata = "cloud-init-userdata"
	)

	var (
		bsArgs     vmlifecycle.BootstrapArgs
		configInfo *vimtypes.VirtualMachineConfigInfo

		metaData string
		userData string
	)

	BeforeEach(func() {
		configInfo = &vimtypes.VirtualMachineConfigInfo{}
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
			configSpec *vimtypes.VirtualMachineConfigSpec
			custSpec   *vimtypes.CustomizationSpec
			err        error

			vmCtx         pkgctx.VirtualMachineContext
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

			vmCtx = pkgctx.VirtualMachineContext{
				Context: context.Background(),
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

		Context("Pending network changes because VM is powered on", func() {
			BeforeEach(func() {
				bsArgs.NetworkResults.UpdatedEthCards = true
				vmCtx.MoVM.Runtime.PowerState = vimtypes.VirtualMachinePowerStatePoweredOn
			})

			It("returns no error", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).To(BeNil())
				Expect(custSpec).To(BeNil())
			})
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
				BeforeEach(func() {
					// Assert vAppConfig removal in this test too.
					configInfo.VAppConfig = &vimtypes.VmConfigInfo{}
				})

				It("Should return valid data", func() {
					Expect(custSpec).To(BeNil())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
					Expect(*configSpec.VAppConfigRemoved).To(BeTrue())

					extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
					act, err := pkgutil.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
					Expect(err).ToNot(HaveOccurred())
					exp, err := pkgutil.TryToDecodeBase64Gzip([]byte("H4sIADOdWGYAA03MSQ6EIBBA0X2doiJrex5sLmNQygZTAoEyXL9Jr1z/vK8UCm2JjZDG1YfVgJo57rafY1j8F2AvlIuGHp0pjuyYTCnVauwu19v98Xy9h08HiMFs7TDF6VQ9lxigZi80Lp7pr9tOKIjGGjPbBpIRp/HsiDkeuzjKdOgefimuS0mkAAAA"))
					Expect(err).ToNot(HaveOccurred())
					Expect(act).To(Equal(exp))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))
				})
			})

			Context("With default user", func() {
				BeforeEach(func() {
					cloudInitSpec.CloudConfig.DefaultUserEnabled = true
				})
				It("Should return valid data", func() {
					Expect(custSpec).To(BeNil())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).To(BeNil())

					extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
					act, err := pkgutil.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
					Expect(err).ToNot(HaveOccurred())
					exp, err := pkgutil.TryToDecodeBase64Gzip([]byte("H4sIAH2dWGYAA03MSw6DMAxF0blXYcGY/j80m0GGmCbIJCgxyvYbdcTsSVfvtC0qr5uQssHFh4WgnSTutptimP0XYM+csoEOLc+0i9blKDu2w0Y5F2uwuVxv98fz9e4/DSAGWqs1xvFUvOQYoCSvPMxe+O9UWDmowRKT2HrYSJ3Bs2OReOzqOPGhe/gBk57Cza4AAAA="))
					Expect(err).ToNot(HaveOccurred())
					Expect(act).To(Equal(exp))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))
				})
			})

			Context("With runcmds", func() {
				BeforeEach(func() {
					cloudInitSpec.CloudConfig.RunCmd = []byte(`["ls /",["ls","-a","-l","/"],["echo","hello, world."]]`)
				})
				It("Should return valid data", func() {
					Expect(custSpec).To(BeNil())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).To(BeNil())

					extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
					act, err := pkgutil.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
					Expect(err).ToNot(HaveOccurred())
					exp, err := pkgutil.TryToDecodeBase64Gzip([]byte("H4sIANedWGYAA02OSw6EIBBE932Kjm7Hcf4fLmMQ2gHTggGM1x/EjatXnUpVV11jomlmmUjgaN0ooVbsF90o7wb7AwiLU5MW0CBHbDM2AZjRyB1csFukjC+nIWZ/wtUH1mdYIoW4dRgZDeluljGuWmB1ud7uj+fr/flWOebklGf0vj+vlqN3sAabqBssU0nnTYlcEnttDswyGYFteXb0k6FAB9/CHw2+gEbpAAAA"))
					Expect(err).ToNot(HaveOccurred())
					Expect(act).To(Equal(exp))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))
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
					Expect(configSpec.VAppConfigRemoved).To(BeNil())

					extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
					Expect(extraConfig).To(HaveLen(4))
					Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata)) // TODO: Better assertion (reduce w/ GetCloudInitMetadata)
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
					Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))

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

						extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
						Expect(extraConfig).To(HaveLen(4))
						Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoMetadata)) // TODO: Better assertion (reduce w/ GetCloudInitMetadata)
						Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))

						Expect(extraConfig).To(HaveKey(constants.CloudInitGuestInfoUserdata))
						Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))

						data, err := pkgutil.TryToDecodeBase64Gzip([]byte(extraConfig[constants.CloudInitGuestInfoUserdata]))
						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(Equal(otherUserData))
					})
				})
			})
		})
	})

	Context("GetCloudInitMetadata", func() {
		var (
			uid            string
			hostName       string
			domainName     string
			netPlan        *netplan.Network
			sshPublicKeys  string
			waitOnNetwork4 *bool
			waitOnNetwork6 *bool

			mdYaml string
			err    error
		)

		BeforeEach(func() {
			uid = "my-uid"
			hostName = "my-hostname"
			domainName = ""
			netPlan = &netplan.Network{
				Version: 42,
				Ethernets: map[string]netplan.Ethernet{
					"eth0": {
						SetName: ptr.To("eth0"),
					},
				},
			}
			sshPublicKeys = "my-ssh-key"
			waitOnNetwork4 = nil
			waitOnNetwork6 = nil
		})

		JustBeforeEach(func() {
			mdYaml, err = vmlifecycle.GetCloudInitMetadata(uid, hostName, domainName, netPlan, sshPublicKeys, waitOnNetwork4, waitOnNetwork6)
		})

		When("domainName is empty", func() {
			BeforeEach(func() {
				domainName = ""
			})
			It("DoIt", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mdYaml).ToNot(BeEmpty())

				ciMetadata := &vmlifecycle.CloudInitMetadata{}
				Expect(yaml.Unmarshal([]byte(mdYaml), ciMetadata)).To(Succeed())

				Expect(ciMetadata.InstanceID).To(Equal(uid))
				Expect(ciMetadata.Hostname).To(Equal(hostName))
				Expect(ciMetadata.PublicKeys).To(Equal(sshPublicKeys))
				Expect(ciMetadata.Network.Version).To(BeEquivalentTo(42))
				Expect(ciMetadata.Network.Ethernets).To(HaveKey("eth0"))
			})
		})

		When("domainName is non-empty", func() {
			BeforeEach(func() {
				domainName = "local"
			})
			It("DoIt", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mdYaml).ToNot(BeEmpty())

				ciMetadata := &vmlifecycle.CloudInitMetadata{}
				Expect(yaml.Unmarshal([]byte(mdYaml), ciMetadata)).To(Succeed())

				Expect(ciMetadata.InstanceID).To(Equal(uid))
				Expect(ciMetadata.Hostname).To(Equal(hostName + "." + domainName))
				Expect(ciMetadata.PublicKeys).To(Equal(sshPublicKeys))
				Expect(ciMetadata.Network.Version).To(BeEquivalentTo(42))
				Expect(ciMetadata.Network.Ethernets).To(HaveKey("eth0"))
			})
		})

		When("wait-on-network is configured", func() {
			BeforeEach(func() {
				waitOnNetwork4 = ptr.To(true)
				waitOnNetwork6 = ptr.To(true)
			})
			It("includes wait-on-network in metadata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(mdYaml).ToNot(BeEmpty())

				ciMetadata := &vmlifecycle.CloudInitMetadata{}
				Expect(yaml.Unmarshal([]byte(mdYaml), ciMetadata)).To(Succeed())

				Expect(ciMetadata.WaitOnNetwork).ToNot(BeNil())
				Expect(ciMetadata.WaitOnNetwork.IPv4).To(BeTrue())
				Expect(ciMetadata.WaitOnNetwork.IPv6).To(BeTrue())
			})
		})

	})

	Context("GetCloudInitGuestInfoCustSpec", func() {
		var (
			configSpec *vimtypes.VirtualMachineConfigSpec
			err        error
		)

		JustBeforeEach(func() {
			configSpec, err = vmlifecycle.GetCloudInitGuestInfoCustSpec(
				context.Background(), configInfo, metaData, userData)
		})

		Context("vAppConfig", func() {

			Context("Config has vAppConfig", func() {
				BeforeEach(func() {
					configInfo.VAppConfig = &vimtypes.VmConfigInfo{}
				})

				It("ConfigSpec should disable vAppConfig", func() {
					Expect(err).ToNot(HaveOccurred())

					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
					Expect(*configSpec.VAppConfigRemoved).To(BeTrue())
				})
			})

			Context("Config does not have vAppConfig", func() {
				BeforeEach(func() {
					configInfo.VAppConfig = nil
				})

				It("ConfigSpec does not set remove vAppConfig field", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfigRemoved).To(BeNil())
				})
			})
		})

		Context("No userdata", func() {
			BeforeEach(func() {
				userData = ""
			})

			It("ConfigSpec.ExtraConfig to only have metadata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
				Expect(extraConfig).To(HaveLen(2))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
			})
		})

		Context("With userdata", func() {
			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))
			})
		})

		Context("With base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				userData = base64.StdEncoding.EncodeToString([]byte(cloudInitUserdata))
			})

			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))
			})
		})

		Context("With gzipped, base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				data, err := pkgutil.EncodeGzipBase64(cloudInitUserdata)
				Expect(err).ToNot(HaveOccurred())
				userData = data
			})

			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).ToNot(BeNil())

				extraConfig := pkgutil.OptionValues(configSpec.ExtraConfig).StringMap()
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoMetadataEncoding, "gzip+base64"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdata, "H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig).To(HaveKeyWithValue(constants.CloudInitGuestInfoUserdataEncoding, "gzip+base64"))
			})
		})
	})

	Context("GetCloudInitPrepCustSpec", func() {
		var (
			configSpec *vimtypes.VirtualMachineConfigSpec
			custSpec   *vimtypes.CustomizationSpec
		)

		JustBeforeEach(func() {
			var err error
			configSpec, custSpec, err = vmlifecycle.GetCloudInitPrepCustSpec(
				context.Background(), configInfo, metaData, userData)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("vAppConfig", func() {

			Context("Config does not have vAppConfig", func() {
				BeforeEach(func() {
					configInfo.VAppConfig = nil
				})

				It("ConfigSpec has expected vAppConfig", func() {
					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfig).ToNot(BeNil())
					Expect(configSpec.VAppConfig.GetVmConfigSpec()).ToNot(BeNil())
					Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport).To(HaveLen(1))
					Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport[0]).To(Equal(vmlifecycle.OvfEnvironmentTransportGuestInfo))
				})
			})

			Context("Config already has a vAppConfig", func() {
				BeforeEach(func() {
					configInfo.VAppConfig = &vimtypes.VmConfigInfo{}
				})

				It("ConfigSpec updates to expected vAppConfig", func() {
					Expect(configSpec).ToNot(BeNil())
					Expect(configSpec.VAppConfig).ToNot(BeNil())
					Expect(configSpec.VAppConfig.GetVmConfigSpec()).ToNot(BeNil())
					Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport).To(HaveLen(1))
					Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport[0]).To(Equal(vmlifecycle.OvfEnvironmentTransportGuestInfo))
				})
			})

			Context("Config already has expected vAppConfig", func() {
				BeforeEach(func() {
					configInfo.VAppConfig = &vimtypes.VmConfigInfo{
						OvfEnvironmentTransport: []string{vmlifecycle.OvfEnvironmentTransportGuestInfo},
					}
				})

				It("ConfigSpec is nil", func() {
					Expect(configSpec).To(BeNil())
				})
			})
		})

		Context("With no userdata", func() {
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

		Context("With metadata and userdata", func() {

			It("Cust spec to have metadata", func() {
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
				data, err := pkgutil.EncodeGzipBase64(cloudInitUserdata)
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
