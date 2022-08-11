// Copyright (c) 2021-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	goctx "context"
	"encoding/base64"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
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
			updateArgs.VMMetadata = vmprovider.VMMetadata{
				Data:      make(map[string]string),
				Transport: vmopv1alpha1.VirtualMachineMetadataOvfEnvTransport,
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
							UserConfigurable: pointer.BoolPtr(true),
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
			updateArgs.VMMetadata = vmprovider.VMMetadata{
				Data:      make(map[string]string),
				Transport: vmopv1alpha1.VirtualMachineMetadataExtraConfigTransport,
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
		updateArgs.DNSServers = []string{nameserver}
		updateArgs.NetIfList = []network.InterfaceInfo{
			{Customization: customizationAdaptorMapping},
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

})

var _ = Describe("CloudInitmetadata", func() {
	var (
		vm             *vmopv1alpha1.VirtualMachine
		netplan        network.Netplan
		metadataString string
		err            error
		updateArgs     session.VMUpdateArgs
		publicKeys     string
	)
	BeforeEach(func() {
		vm = &vmopv1alpha1.VirtualMachine{
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
				data, err := session.EncodeGzipBase64(cloudInitUserdata)
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
			err      error
			custSpec *vimTypes.CustomizationSpec
		)
		JustBeforeEach(func() {
			custSpec, err = session.GetCloudInitPrepCustSpec(cloudInitMetadata, updateArgs)
		})

		Context("No userdata", func() {
			It("Cust spec to only have metadata", func() {
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())
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
				Expect(err).ToNot(HaveOccurred())
				Expect(custSpec).ToNot(BeNil())
				cloudinitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudinitPrepSpec.Userdata).To(Equal("cloud-init-userdata"))
			})
		})

		Context("With gzipped, base64-encoded userdata but no encoding specified", func() {
			BeforeEach(func() {
				data, err := session.EncodeGzipBase64(cloudInitUserdata)
				Expect(err).ToNot(HaveOccurred())
				updateArgs.VMMetadata.Data["user-data"] = data
			})
			It("Cust spec to have metadata and userdata", func() {
				Expect(err).ToNot(HaveOccurred())
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
			updateArgs  session.VMUpdateArgs
			ip1         = "192.168.1.37"
			ip2         = "192.168.10.48"
			subnetMask1 = "255.0.0.0"
			subnetMask2 = "255.255.255.0"
			IP1         = "192.168.1.37/8"
			IP2         = "192.168.10.48/24"
			gateway1    = "192.168.1.1"
			gateway2    = "192.168.10.1"
			nameserver  = "8.8.8.8"
		)

		vm := &vmopv1alpha1.VirtualMachine{
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

		BeforeEach(func() {
			updateArgs.DNSServers = []string{nameserver}
			updateArgs.NetIfList = []network.InterfaceInfo{
				{
					IPConfiguration: network.IPConfig{
						Gateway:    gateway1,
						IP:         ip1,
						SubnetMask: subnetMask1,
					},
				},
				{
					IPConfiguration: network.IPConfig{
						Gateway:    gateway2,
						IP:         ip2,
						SubnetMask: subnetMask2,
					},
				},
			}
			updateArgs.VMMetadata = vmprovider.VMMetadata{
				Data: make(map[string]string),
			}
		})

		It("should return populated NicInfoToNetworkIfStatusEx correctly", func() {
			networkDevicesStatus := session.NicInfoToDevicesStatus(vmCtx, updateArgs)
			Expect(networkDevicesStatus[0].IPAddresses[0]).To(Equal(IP1))
			Expect(networkDevicesStatus[0].Gateway4).To(Equal(gateway1))
			Expect(networkDevicesStatus[1].IPAddresses[0]).To(Equal(IP2))
			Expect(networkDevicesStatus[1].Gateway4).To(Equal(gateway2))
		})

		It("should resolve them correctly while specifying valid templates", func() {

			updateArgs.VMMetadata.Data["first_ip"] = "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"
			updateArgs.VMMetadata.Data["second_ip"] = "{{ (index (index .V1alpha1.Net.Devices 1).IPAddresses 0) }}"
			updateArgs.VMMetadata.Data["first_gateway"] = "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}"
			updateArgs.VMMetadata.Data["second_gateway"] = "{{ (index .V1alpha1.Net.Devices 1).Gateway4 }}"
			updateArgs.VMMetadata.Data["nameserver"] = "{{ (index .V1alpha1.Net.Nameservers 0) }}"
			updateArgs.VMMetadata.Data["name"] = "{{ .V1alpha1.VM.Name }}"

			session.TemplateVMMetadata(vmCtx, updateArgs)
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("first_ip", IP1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("second_ip", IP2))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("first_gateway", gateway1))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("second_gateway", gateway2))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameserver", nameserver))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("name", "dummy-vm"))
		})

		It("should use the original text if resolving template failed", func() {
			updateArgs.VMMetadata.Data["ip"] = "{{ (index .V1alpha1.Net.NetworkInterfaces 100).IPAddresses }}"
			updateArgs.VMMetadata.Data["gateway"] = "{{ (index .V1alpha1.Net.NetworkInterfaces ).Gateway }}"
			updateArgs.VMMetadata.Data["nameserver"] = "{{ (index .V1alpha1.Net.NameServers 0) }}"

			session.TemplateVMMetadata(vmCtx, updateArgs)

			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("ip", "{{ (index .V1alpha1.Net.NetworkInterfaces 100).IPAddresses }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("gateway", "{{ (index .V1alpha1.Net.NetworkInterfaces ).Gateway }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("nameserver", "{{ (index .V1alpha1.Net.NameServers 0) }}"))
		})

		It("should escape parsing when escape character used", func() {
			updateArgs.VMMetadata.Data["skip_data1"] = "\\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}"
			updateArgs.VMMetadata.Data["skip_data2"] = "\\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"
			updateArgs.VMMetadata.Data["skip_data3"] = "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}"
			updateArgs.VMMetadata.Data["skip_data4"] = "skip \\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}"

			session.TemplateVMMetadata(vmCtx, updateArgs)

			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data1", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data2", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data3", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
			Expect(updateArgs.VMMetadata.Data).To(HaveKeyWithValue("skip_data4", "skip {{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"))
		})
	})
})
