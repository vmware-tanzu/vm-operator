// Copyright (c) 2021-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	goctx "context"
	"encoding/base64"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	kyaml "sigs.k8s.io/yaml"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("Customization utils", func() {
	Context("IsPending", func() {
		var extraConfig []vimTypes.BaseOptionValue
		var pending bool

		BeforeEach(func() {
			extraConfig = nil
		})

		JustBeforeEach(func() {
			pending = session.IsCustomizationPendingExtraConfig(extraConfig)
		})

		Context("Empty ExtraConfig", func() {
			It("not pending", func() {
				Expect(pending).To(BeFalse())
			})
		})

		Context("ExtraConfig with pending key", func() {
			BeforeEach(func() {
				extraConfig = append(extraConfig, &vimTypes.OptionValue{
					Key:   constants.GOSCPendingExtraConfigKey,
					Value: "/foo/bar",
				})
			})

			It("is pending", func() {
				Expect(pending).To(BeTrue())
			})
		})
	})
})

var _ = Describe("Customization via ConfigSpec", func() {
	var (
		updateArgs session.VMUpdateArgs
		configInfo *vimTypes.VirtualMachineConfigInfo
		configSpec *vimTypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		configInfo = &vimTypes.VirtualMachineConfigInfo{}
	})

	Context("GetOvfEnvCustSpec", func() {
		BeforeEach(func() {
			updateArgs.VMMetadata = session.VMMetadata{
				Data:      make(map[string]string),
				Transport: vmopv1.VirtualMachineMetadataOvfEnvTransport,
			}
		})
		JustBeforeEach(func() {
			configSpec = session.GetOvfEnvCustSpec(configInfo, updateArgs)
		})
		Context("Empty input", func() {
			It("No changes", func() {
				Expect(configSpec).To(BeNil())
			})
		})
		Context("Update to user configurable field", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["foo"] = "fooval"
				configInfo.VAppConfig = &vimTypes.VmConfigInfo{
					Property: []vimTypes.VAppPropertyInfo{
						{
							Id:               "foo",
							Value:            "should-change",
							UserConfigurable: pointer.Bool(true),
						},
					},
				}
			})
			It("Updates configSpec.VAppConfig", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(configSpec.VAppConfig).ToNot(BeNil())
				vmCs := configSpec.VAppConfig.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Value).To(Equal("fooval"))
			})
		})
	})

	Context("GetExtraConfigCustSpec", func() {
		BeforeEach(func() {
			updateArgs.VMMetadata = session.VMMetadata{
				Data:      make(map[string]string),
				Transport: vmopv1.VirtualMachineMetadataExtraConfigTransport,
			}
		})
		JustBeforeEach(func() {
			configSpec = session.GetExtraConfigCustSpec(configInfo, updateArgs)
		})
		Context("Empty input", func() {
			It("No changes", func() {
				Expect(configSpec).To(BeNil())
			})
		})
		Context("Invalid guestinfo key", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["nonguestinfokey"] = "nonguestinfovalue"
			})
			It("No changes", func() {
				Expect(configSpec).To(BeNil())
			})
		})
		Context("Mixed valid and invalid guestinfo keys", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["guestinfo.foo"] = "bar"
				updateArgs.VMMetadata.Data["guestinfo.hello"] = "world"
				updateArgs.VMMetadata.Data["nonguestinfokey"] = "nonguestinfovalue"
			})
			It("Updates configSpec.ExtraConfig", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(configSpec.ExtraConfig).To(HaveLen(2))
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig["guestinfo.foo"]).To(Equal("bar"))
				Expect(extraConfig["guestinfo.hello"]).To(Equal("world"))
			})
		})
	})
})

var _ = Describe("Customization via Cust Spec", func() {
	var (
		err        error
		updateArgs session.VMUpdateArgs
		macaddress = "01-23-45-67-89-AB-CD-EF"
		nameserver = "8.8.8.8"
		vmName     = "dummy-vm"
		custSpec   *vimTypes.CustomizationSpec
	)

	customizationAdaptorMapping := &vimTypes.CustomizationAdapterMapping{
		MacAddress: macaddress,
	}

	BeforeEach(func() {
		updateArgs = session.VMUpdateArgs{
			DNSServers: []string{nameserver},
			NetIfList: []network.InterfaceInfo{
				{Customization: customizationAdaptorMapping},
			},
		}
	})

	Context("GetLinuxPrepCustSpec", func() {
		JustBeforeEach(func() {
			custSpec = session.GetLinuxPrepCustSpec(vmName, updateArgs)
		})
		It("should return linux customization spec", func() {
			Expect(custSpec).ToNot(BeNil())
			Expect(custSpec.GlobalIPSettings.DnsServerList).To(Equal(updateArgs.DNSServers))
			Expect(custSpec.NicSettingMap).To(Equal([]vimTypes.CustomizationAdapterMapping{*customizationAdaptorMapping}))
			linuxSpec := custSpec.Identity.(*vimTypes.CustomizationLinuxPrep)
			hostName := linuxSpec.HostName.(*vimTypes.CustomizationFixedName).Name
			Expect(hostName).To(Equal(vmName))
		})
	})

	Context("GetSysprepCustSpec", func() {
		const unattendXML = "dummy-unattend-xml"

		BeforeEach(func() {
			updateArgs.VMMetadata.Data = map[string]string{
				"unattend": unattendXML,
			}
		})
		JustBeforeEach(func() {
			custSpec, err = session.GetSysprepCustSpec(vmName, updateArgs)
			Expect(err).ToNot(HaveOccurred())
		})
		It("should return sysprepText customization spec", func() {
			Expect(custSpec).ToNot(BeNil())
			Expect(custSpec.GlobalIPSettings.DnsServerList).To(Equal(updateArgs.DNSServers))
			Expect(custSpec.NicSettingMap).To(Equal([]vimTypes.CustomizationAdapterMapping{*customizationAdaptorMapping}))
			sysprepSpec := custSpec.Identity.(*vimTypes.CustomizationSysprepText)
			Expect(sysprepSpec.Value).To(Equal(unattendXML))
		})
	})

})

var _ = Describe("CloudInitmetadata", func() {
	var (
		vm             *vmopv1.VirtualMachine
		netplan        network.Netplan
		metadataString string
		err            error
		updateArgs     session.VMUpdateArgs
		publicKeys     string
	)
	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
				UID:       "dummy-id",
			},
		}
		netplan = network.Netplan{
			Version: constants.NetPlanVersion,
			Ethernets: map[string]network.NetplanEthernet{
				"eth0": {
					Dhcp4: false,
					Match: network.NetplanEthernetMatch{
						MacAddress: "00:50:56:9b:3a:67",
					},
					Addresses: []string{"192.168.1.55"},
					Gateway4:  "192.168.1.1",
					Nameservers: network.NetplanEthernetNameserver{
						Addresses: []string{"8.8.8.8", "1.1.1.1"},
						Search:    []string{"vmware.com", "example.com"},
					},
				},
			},
		}
		publicKeys = "dummy-ssh-public-key"
		updateArgs.VMMetadata.Data = map[string]string{}
		updateArgs.VMMetadata.Data["ssh-public-keys"] = publicKeys
	})
	JustBeforeEach(func() {
		metadataString, err = session.GetCloudInitMetadata(vm, netplan, updateArgs.VMMetadata.Data)
	})

	It("Return a valid cloud-init metadata yaml", func() {
		Expect(metadataString).ToNot(BeEmpty())
		Expect(err).ToNot(HaveOccurred())

		metadata := session.CloudInitMetadata{}
		err = yaml.Unmarshal([]byte(metadataString), &metadata)
		Expect(err).ToNot(HaveOccurred())

		Expect(metadata.InstanceID).To(Equal(string(vm.UID)))
		Expect(metadata.LocalHostname).To(Equal(vm.Name))
		Expect(metadata.Hostname).To(Equal(vm.Name))
		Expect(metadata.Network).To(Equal(netplan))
		Expect(metadata.PublicKeys).To(Equal(publicKeys))
	})
})

var _ = Describe("Cloud-Init Customization", func() {
	var (
		cloudInitMetadata string
		cloudInitUserdata string
		updateArgs        session.VMUpdateArgs
		configInfo        *vimTypes.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		cloudInitMetadata = "cloud-init-metadata"
		cloudInitUserdata = "cloud-init-userdata"
		configInfo = &vimTypes.VirtualMachineConfigInfo{}
		updateArgs.VMMetadata.Data = map[string]string{}
	})

	Context("GetCloudInitGuestInfoCustSpec", func() {
		var (
			configSpec *vimTypes.VirtualMachineConfigSpec
			err        error
		)
		JustBeforeEach(func() {
			configSpec, err = session.GetCloudInitGuestInfoCustSpec(cloudInitMetadata, configInfo, updateArgs)
		})

		Context("VAppConfig Disabled", func() {
			It("Should disable the VAppConfig", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec.VAppConfigRemoved).ToNot(BeNil())
				Expect(*configSpec.VAppConfigRemoved).To(BeTrue())
			})
		})

		Context("No userdata", func() {
			It("ConfigSpec.ExtraConfig to only have metadata", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(2))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With userdata", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["user-data"] = cloudInitUserdata
			})
			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["user-data"] = base64.StdEncoding.EncodeToString([]byte(cloudInitUserdata))
			})
			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
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
				updateArgs.VMMetadata.Data["user-data"] = data
			})
			It("ConfigSpec.ExtraConfig to have metadata and userdata", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With userdata in both a 'user-data' and a 'value'  key", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["user-data"] = cloudInitUserdata
				updateArgs.VMMetadata.Data["value"] = cloudInitUserdata + "-in-value"
			})
			It("the 'user-data' key overrides as the ConfigSpec.ExtraConfig userdata ", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
				Expect(extraConfig).To(HaveLen(4))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRzU0tSUxJLEkEAAAA//8BAAD//wEq0o4TAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoMetadataEncoding]).To(Equal("gzip+base64"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdata]).To(Equal("H4sIAAAAAAAA/0rOyS9N0c3MyyzRLS1OLUpJLEkEAAAA//8BAAD//weVSMoTAAAA"))
				Expect(extraConfig[constants.CloudInitGuestInfoUserdataEncoding]).To(Equal("gzip+base64"))
			})
		})

		Context("With CAPBK userdata in a 'value' key", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["value"] = cloudInitUserdata
			})
			It("ConfigSpec.ExtraConfig's userdata will have values from the 'value' key", func() {
				Expect(configSpec).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
				extraConfig := session.ExtraConfigToMap(configSpec.ExtraConfig)
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
			custSpec *vimTypes.CustomizationSpec
		)
		JustBeforeEach(func() {
			var (
				err        error
				configSpec *vimTypes.VirtualMachineConfigSpec
			)
			configSpec, custSpec, err = session.GetCloudInitPrepCustSpec(cloudInitMetadata, updateArgs)
			Expect(err).ToNot(HaveOccurred())

			// Validate that Cloud-Init Prep always uses the GuestInfo transport
			// for the CI Prep's meta and user data.
			Expect(configSpec).ToNot(BeNil())
			Expect(configSpec.VAppConfig).ToNot(BeNil())
			Expect(configSpec.VAppConfig.GetVmConfigSpec()).ToNot(BeNil())
			Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport).To(HaveLen(1))
			Expect(configSpec.VAppConfig.GetVmConfigSpec().OvfEnvironmentTransport[0]).To(Equal(session.OvfEnvironmentTransportGuestInfo))
		})

		Context("No userdata", func() {
			It("Cust spec to only have metadata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudinitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudinitPrepSpec.Userdata).To(BeEmpty())
			})
		})

		Context("With userdata", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["user-data"] = "cloud-init-userdata"
			})
			It("Cust spec to have metadata and userdata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudinitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudinitPrepSpec.Userdata).To(Equal("cloud-init-userdata"))
			})
		})

		Context("With base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				updateArgs.VMMetadata.Data["user-data"] = base64.StdEncoding.EncodeToString([]byte(cloudInitUserdata))
			})
			It("Cust spec to have metadata and userdata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudinitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudinitPrepSpec.Userdata).To(Equal("cloud-init-userdata"))
			})
		})

		Context("With gzipped, base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				data, err := util.EncodeGzipBase64(cloudInitUserdata)
				Expect(err).ToNot(HaveOccurred())
				updateArgs.VMMetadata.Data["user-data"] = data
			})
			It("Cust spec to have metadata and userdata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudinitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudinitPrepSpec.Userdata).To(Equal("cloud-init-userdata"))
			})
		})
	})
})

var _ = Describe("TemplateVMMetadata", func() {
	Context("update VmConfigArgs", func() {
		var (
			updateArgs      session.VMUpdateArgs
			ip1             = "192.168.1.37"
			ip2             = "192.168.10.48"
			subnetMask      = "255.255.255.0"
			IP1             = "192.168.1.37/24"
			IP2             = "192.168.10.48/24"
			gateway1        = "192.168.1.1"
			gateway2        = "192.168.10.1"
			nameserver1     = "8.8.8.8"
			nameserver2     = "1.1.1.1"
			existingMacAddr = "00-00-00-00-00-00"
			firstNicMacAddr string
			vm              *vmopv1.VirtualMachine
			vmCtx           context.VirtualMachineContext
			ctx             *builder.TestContextForVCSim
			resVM           *res.VirtualMachine
		)

		BeforeEach(func() {
			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-vm",
					Namespace: "dummy-ns",
				},
			}
			vmCtx = context.VirtualMachineContext{
				Context: goctx.Background(),
				Logger:  logf.Log.WithValues("vmName", vm.NamespacedName()),
				VM:      vm,
			}
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			objVM, err := ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
			Expect(err).ToNot(HaveOccurred())
			resVM = res.NewVMFromObject(objVM)

			// Get the first NIC's MAC address of the above VM.
			deviceList, err := objVM.Device(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			ethCards := deviceList.SelectByType((*vimTypes.VirtualEthernetCard)(nil))
			Expect(ethCards).ToNot(BeEmpty())
			nic := ethCards[0].(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			firstNicMacAddr = nic.GetVirtualEthernetCard().MacAddress
			firstNicMacAddr = strings.ReplaceAll(firstNicMacAddr, ":", "-")

			updateArgs.DNSServers = []string{nameserver1, nameserver2}
			updateArgs.NetIfList = []network.InterfaceInfo{
				{
					IPConfiguration: network.IPConfig{
						Gateway:    gateway1,
						IP:         ip1,
						SubnetMask: subnetMask,
					},
					// No customization is set here to test getting the first Nic's MAC address.
				},
				{
					IPConfiguration: network.IPConfig{
						Gateway:    gateway2,
						IP:         ip2,
						SubnetMask: subnetMask,
					},
					Customization: &vimTypes.CustomizationAdapterMapping{
						MacAddress: existingMacAddr,
					},
				},
			}
			updateArgs.VMMetadata = session.VMMetadata{
				Data: make(map[string]string),
			}
		})

		It("should return populated NicInfoToNetworkIfStatusEx correctly", func() {
			IPs := []string{IP1}
			networkDevicesStatus := session.NicInfoToDevicesStatus(vmCtx, resVM, updateArgs)
			Expect(networkDevicesStatus[0].IPAddresses).To(Equal(IPs))
			Expect(networkDevicesStatus[0].IPAddresses[0]).To(Equal(IP1))
			Expect(networkDevicesStatus[0].Gateway4).To(Equal(gateway1))
			Expect(networkDevicesStatus[0].MacAddress).To(Equal(firstNicMacAddr))
			Expect(networkDevicesStatus[1].IPAddresses[0]).To(Equal(IP2))
			Expect(networkDevicesStatus[1].Gateway4).To(Equal(gateway2))
			Expect(networkDevicesStatus[1].MacAddress).To(Equal(existingMacAddr))
		})

		It("should resolve them correctly while specifying valid templates", func() {

			updateArgs.VMMetadata.Data["first_cidrIp"] = "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"
			updateArgs.VMMetadata.Data["second_cidrIp"] = "{{ (index (index .V1alpha1.Net.Devices 1).IPAddresses 0) }}"
			updateArgs.VMMetadata.Data["first_gateway"] = "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
			updateArgs.VMMetadata.Data["second_gateway"] = "{{ (index .V1alpha1.Net.Devices 1).Gateway4 }}"
			updateArgs.VMMetadata.Data["nameserver"] = "{{ (index .V1alpha1.Net.Nameservers 0) }}"
			updateArgs.VMMetadata.Data["name"] = "{{ .V1alpha1.VM.Name }}"
			updateArgs.VMMetadata.Data["existingMacAddr"] = "{{ (index .V1alpha1.Net.Devices 1).MacAddress }}"

			session.TemplateVMMetadata(vmCtx, resVM, updateArgs)
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("first_cidrIp", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("second_cidrIp", IP2))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("first_gateway", gateway1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("second_gateway", gateway2))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameserver", nameserver1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("name", "dummy-vm"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("existingMacAddr", existingMacAddr))
		})

		It("should resolve them correctly while using support template queries", func() {
			updateArgs.VMMetadata.Data["cidr_ip1"] = "{{ " + constants.V1alpha1FirstIP + " }}"
			updateArgs.VMMetadata.Data["cidr_ip2"] = "{{ " + constants.V1alpha1FirstIPFromNIC + " 1 }}"
			updateArgs.VMMetadata.Data["cidr_ip3"] = "{{ (" + constants.V1alpha1IP + " \"192.168.1.37\") }}"
			updateArgs.VMMetadata.Data["cidr_ip4"] = "{{ (" + constants.V1alpha1FormatIP + " \"192.168.1.37\" \"/24\") }}"
			updateArgs.VMMetadata.Data["cidr_ip5"] = "{{ (" + constants.V1alpha1FormatIP + " \"192.168.1.37\" \"255.255.255.0\") }}"
			updateArgs.VMMetadata.Data["cidr_ip6"] = "{{ (" + constants.V1alpha1FormatIP + " \"192.168.1.37/28\" \"255.255.255.0\") }}"
			updateArgs.VMMetadata.Data["cidr_ip7"] = "{{ (" + constants.V1alpha1FormatIP + " \"192.168.1.37/28\" \"/24\") }}"
			updateArgs.VMMetadata.Data["ip1"] = "{{ " + constants.V1alpha1FormatIP + " " + constants.V1alpha1FirstIP + " \"\" }}"
			updateArgs.VMMetadata.Data["ip2"] = "{{ " + constants.V1alpha1FormatIP + " \"192.168.1.37/28\" \"\" }}"
			updateArgs.VMMetadata.Data["ips_1"] = "{{ " + constants.V1alpha1IPsFromNIC + " 0 }}"
			updateArgs.VMMetadata.Data["subnetmask"] = "{{ " + constants.V1alpha1SubnetMask + " \"192.168.1.37/26\" }}"
			updateArgs.VMMetadata.Data["firstNicMacAddr"] = "{{ " + constants.V1alpha1FirstNicMacAddr + " }}"
			updateArgs.VMMetadata.Data["formatted_nameserver1"] = "{{ " + constants.V1alpha1FormatNameservers + " 1 \"-\"}}"
			updateArgs.VMMetadata.Data["formatted_nameserver2"] = "{{ " + constants.V1alpha1FormatNameservers + " -1 \"-\"}}"
			session.TemplateVMMetadata(vmCtx, resVM, updateArgs)

			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip1", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip2", IP2))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip3", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip4", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip5", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip6", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip7", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip1", ip1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip2", ip1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ips_1", fmt.Sprint([]string{IP1})))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("subnetmask", "255.255.255.192"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("firstNicMacAddr", firstNicMacAddr))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("formatted_nameserver1", nameserver1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("formatted_nameserver2", nameserver1+"-"+nameserver2))
		})

		It("should use the original text if resolving template failed", func() {
			updateArgs.VMMetadata.Data["ip1"] = "{{ " + constants.V1alpha1IP + " \"192.1.0\" }}"
			updateArgs.VMMetadata.Data["ip2"] = "{{ " + constants.V1alpha1FirstIPFromNIC + " 5 }}"
			updateArgs.VMMetadata.Data["ips_1"] = "{{ " + constants.V1alpha1IPsFromNIC + " 5 }}"
			updateArgs.VMMetadata.Data["cidr_ip1"] = "{{ (" + constants.V1alpha1FormatIP + " \"192.168.1.37\" \"127.255.255.255\") }}"
			updateArgs.VMMetadata.Data["cidr_ip2"] = "{{ (" + constants.V1alpha1FormatIP + " \"192.168.1\" \"255.0.0.0\") }}"
			updateArgs.VMMetadata.Data["gateway"] = "{{ (index .V1alpha1.Net.NetworkInterfaces ).Gateway }}"
			updateArgs.VMMetadata.Data["nameserver"] = "{{ (index .V1alpha1.Net.NameServers 0) }}"

			session.TemplateVMMetadata(vmCtx, resVM, updateArgs)

			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip1", "{{ "+constants.V1alpha1IP+" \"192.1.0\" }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip2", "{{ "+constants.V1alpha1FirstIPFromNIC+" 5 }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ips_1", "{{ "+constants.V1alpha1IPsFromNIC+" 5 }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip1", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("cidr_ip2", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("gateway", "{{ (index .V1alpha1.Net.NetworkInterfaces ).Gateway }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameserver", "{{ (index .V1alpha1.Net.NameServers 0) }}"))
		})

		It("should escape parsing when escape character used", func() {
			updateArgs.VMMetadata.Data["skip_data1"] = "\\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}"
			updateArgs.VMMetadata.Data["skip_data2"] = "\\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"
			updateArgs.VMMetadata.Data["skip_data3"] = "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}"
			updateArgs.VMMetadata.Data["skip_data4"] = "skip \\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}"

			session.TemplateVMMetadata(vmCtx, resVM, updateArgs)

			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data1", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data2", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data3", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data4", "skip {{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
		})

		Context("from Kubernetes resources", func() {

			Context("from ConfigMap", func() {
				var (
					obj     *corev1.ConfigMap
					objYAML string
				)

				BeforeEach(func() {
					obj = &corev1.ConfigMap{}
				})

				JustBeforeEach(func() {
					Expect(kyaml.Unmarshal([]byte(objYAML), obj)).To(Succeed())
					for k, v := range obj.Data {
						updateArgs.VMMetadata.Data[k] = v
					}
					session.TemplateVMMetadata(vmCtx, resVM, updateArgs)
				})

				Context("with single-quoted values", func() {
					BeforeEach(func() {
						objYAML = testConfigMapYAML1
					})
					It("should work", func() {
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("hostname", vmCtx.VM.Name))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("management_gateway", gateway1))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("management_ip", ip1))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameservers", nameserver1))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("subnetMask", subnetMask))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nicMacAddr", firstNicMacAddr))
					})
				})
				Context("with double-quoted values and escaped double-quotes", func() {
					BeforeEach(func() {
						objYAML = testConfigMapYAML2
					})
					It("should work", func() {
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("hostname", vmCtx.VM.Name))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("management_gateway", gateway1))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("management_ip", ip1))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameservers", nameserver1))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("subnetMask", subnetMask))
						Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nicMacAddr", firstNicMacAddr))
					})
				})
			})

			Context("from Secret", func() {
				var (
					obj     *corev1.Secret
					objYAML string
				)

				BeforeEach(func() {
					obj = &corev1.Secret{}
					objYAML = testSecretYAML1
				})

				JustBeforeEach(func() {
					Expect(kyaml.Unmarshal([]byte(objYAML), obj)).To(Succeed())
				})

				It("should work", func() {
					for k, v := range obj.Data {
						updateArgs.VMMetadata.Data[k] = string(v)
					}
					session.TemplateVMMetadata(vmCtx, resVM, updateArgs)

					Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("hostname", vmCtx.VM.Name))
					Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("management_gateway", gateway1))
					Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("management_ip", ip1))
					Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameservers", nameserver1))
					Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("subnetMask", subnetMask))
					Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nicMacAddr", firstNicMacAddr))
				})
			})
		})
	})

	Context("update VmConfigArgs with empty updateArgs", func() {
		var (
			updateArgs session.VMUpdateArgs
		)
		vm := &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
		}
		vmCtx := context.VirtualMachineContext{
			Context: goctx.Background(),
			Logger:  logf.Log.WithValues("vmName", vm.NamespacedName()),
			VM:      vm,
		}
		ctx := suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		objVM, err := ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())
		resVM := res.NewVMFromObject(objVM)

		updateArgs.VMMetadata = session.VMMetadata{
			Data: make(map[string]string),
		}

		It("should return the original text when no available network", func() {
			updateArgs.VMMetadata.Data["ip1"] = "{{ " + constants.V1alpha1FirstIP + " }}"
			updateArgs.VMMetadata.Data["ip2"] = "{{ " + constants.V1alpha1FirstIPFromNIC + " 0 }}"
			updateArgs.VMMetadata.Data["ip3"] = "{{ " + constants.V1alpha1IPsFromNIC + " 0 }}"
			updateArgs.VMMetadata.Data["formatted_nameserver1"] = "{{ " + constants.V1alpha1FormatNameservers + " 1 \"-\"}}"

			session.TemplateVMMetadata(vmCtx, resVM, updateArgs)

			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip1", "{{ "+constants.V1alpha1FirstIP+" }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip2", "{{ "+constants.V1alpha1FirstIPFromNIC+" 0 }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip3", "{{ "+constants.V1alpha1IPsFromNIC+" 0 }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("formatted_nameserver1", "{{ "+constants.V1alpha1FormatNameservers+" 1 \"-\"}}"))
		})

	})
})

const testConfigMapYAML1 = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config-map
  namespace: my-namespace
data:
  hostname: '{{ .V1alpha1.VM.Name }}'
  management_gateway: '{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}'
  management_ip: '{{ V1alpha1_FormatIP V1alpha1_FirstIP "" }}'
  nameservers: '{{ (index .V1alpha1.Net.Nameservers 0) }}'
  subnetMask: '{{ V1alpha1_SubnetMask V1alpha1_FirstIP }}'
  nicMacAddr: '{{ V1alpha1_FirstNicMacAddr }}'
`

const testConfigMapYAML2 = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config-map
  namespace: my-namespace
data:
  hostname: "{{ .V1alpha1.VM.Name }}"
  management_gateway: "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
  management_ip: "{{ V1alpha1_FormatIP V1alpha1_FirstIP \"\" }}"
  nameservers: "{{ (index .V1alpha1.Net.Nameservers 0) }}"
  subnetMask: "{{ V1alpha1_SubnetMask V1alpha1_FirstIP }}"
  nicMacAddr: "{{ V1alpha1_FirstNicMacAddr }}"
`

//nolint:gosec
const testSecretYAML1 = `
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
  namespace: my-namespace
data:
  hostname: e3sgLlYxYWxwaGExLlZNLk5hbWUgfX0=
  management_gateway: e3sgKGluZGV4IC5WMWFscGhhMS5OZXQuRGV2aWNlcyAwKS5HYXRld2F5NCB9fQ==
  management_ip: e3sgVjFhbHBoYTFfRm9ybWF0SVAgVjFhbHBoYTFfRmlyc3RJUCAiIiB9fQ==
  nameservers: e3sgKGluZGV4IC5WMWFscGhhMS5OZXQuTmFtZXNlcnZlcnMgMCkgfX0=
  subnetMask: e3sgVjFhbHBoYTFfU3VibmV0TWFzayBWMWFscGhhMV9GaXJzdElQIH19
  nicMacAddr: e3sgVjFhbHBoYTFfRmlyc3ROaWNNYWNBZGRyIH19
`
