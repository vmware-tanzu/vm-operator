// +build !integration

// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	goctx "context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"gopkg.in/yaml.v2"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/context"
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
		vmName         string
		netplan        network.Netplan
		metadataString string
		err            error
	)
	BeforeEach(func() {
		vmName = "SomeVmName"
		netplan = network.Netplan{
			Version: constants.NetPlanVersion,
			Ethernets: map[string]network.NetplanEthernet{
				"nic0": {
					Dhcp4:     false,
					Addresses: []string{"192.168.1.55"},
					Gateway4:  "192.168.1.1",
					Nameservers: network.NetplanEthernetNameserver{
						Addresses: []string{"8.8.8.8", "1.1.1.1"},
					},
				},
			},
		}
	})
	JustBeforeEach(func() {
		metadataString, err = session.GetCloudInitMetadata(vmName, netplan)
	})

	It("Return a valid cloud-init metadata yaml", func() {
		Expect(metadataString).ToNot(BeEmpty())
		Expect(err).ToNot(HaveOccurred())

		metadata := session.CloudInitMetadata{}
		err = yaml.Unmarshal([]byte(metadataString), &metadata)
		Expect(err).ToNot(HaveOccurred())

		Expect(metadata.InstanceID).To(Equal(vmName))
		Expect(metadata.LocalHostname).To(Equal(vmName))
		Expect(metadata.Hostname).To(Equal(vmName))
		Expect(metadata.Network).To(Equal(netplan))
	})
})

var _ = Describe("Cloud-Init Customization", func() {
	var (
		cloudInitMetadata string
		updateArgs        session.VMUpdateArgs
		configInfo        *vimTypes.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		cloudInitMetadata = "cloud-init-metadata"
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
				updateArgs.VMMetadata.Data["user-data"] = "cloud-init-userdata"
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
	})

	Context("GetCloudInitPrepCustSpec", func() {
		var (
			custSpec *vimTypes.CustomizationSpec
		)
		JustBeforeEach(func() {
			custSpec = session.GetCloudInitPrepCustSpec(cloudInitMetadata, updateArgs)
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
			It("Cust spec to only have metadata", func() {
				Expect(custSpec).ToNot(BeNil())
				cloudinitPrepSpec := custSpec.Identity.(*internal.CustomizationCloudinitPrep)
				Expect(cloudinitPrepSpec.Metadata).To(Equal(cloudInitMetadata))
				Expect(cloudinitPrepSpec.Userdata).To(Equal(updateArgs.VMMetadata.Data["user-data"]))
			})
		})
	})
})

var _ = Describe("TemplateVMMetadata", func() {
	Context("update VmConfigArgs", func() {
		var (
			updateArgs session.VMUpdateArgs

			ip         = "192.168.1.37"
			subnetMask = "255.255.255.0"
			gateway    = "192.168.1.1"
			nameserver = "8.8.8.8"
		)

		vm := &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
		}
		vmCtx := context.VMContext{
			Context: goctx.Background(),
			Logger:  logf.Log.WithValues("vmName", vm.NamespacedName()),
			VM:      vm,
		}

		BeforeEach(func() {
			updateArgs.DNSServers = []string{nameserver}
			updateArgs.NetIfList = []network.InterfaceInfo{
				{
					IPConfiguration: network.IPConfig{
						IP:         ip,
						SubnetMask: subnetMask,
						Gateway:    gateway,
					},
				},
			}
			updateArgs.VMMetadata = vmprovider.VMMetadata{
				Data: make(map[string]string),
			}
		})

		It("should resolve them correctly while specifying valid templates", func() {
			updateArgs.VMMetadata.Data["ip"] = "{{ (index .NetworkInterfaces 0).IP }}"
			updateArgs.VMMetadata.Data["subMask"] = "{{ (index .NetworkInterfaces 0).SubnetMask }}"
			updateArgs.VMMetadata.Data["gateway"] = "{{ (index .NetworkInterfaces 0).Gateway }}"
			updateArgs.VMMetadata.Data["nameserver"] = "{{ (index .NameServers 0) }}"

			session.TemplateVMMetadata(vmCtx, updateArgs)

			Expect(updateArgs.VMMetadata.Data["ip"]).To(Equal(ip))
			Expect(updateArgs.VMMetadata.Data["subMask"]).To(Equal(subnetMask))
			Expect(updateArgs.VMMetadata.Data["gateway"]).To(Equal(gateway))
			Expect(updateArgs.VMMetadata.Data["nameserver"]).To(Equal(nameserver))
		})

		It("should use the original text if resolving template failed", func() {
			updateArgs.VMMetadata.Data["ip"] = "{{ (index .NetworkInterfaces 100).IP }}"
			updateArgs.VMMetadata.Data["subMask"] = "{{ invalidTemplate }}"
			updateArgs.VMMetadata.Data["gateway"] = "{{ (index .NetworkInterfaces ).Gateway }}"
			updateArgs.VMMetadata.Data["nameserver"] = "{{ (index .NameServers 0) }}"

			session.TemplateVMMetadata(vmCtx, updateArgs)

			Expect(updateArgs.VMMetadata.Data["ip"]).To(Equal("{{ (index .NetworkInterfaces 100).IP }}"))
			Expect(updateArgs.VMMetadata.Data["subMask"]).To(Equal("{{ invalidTemplate }}"))
			Expect(updateArgs.VMMetadata.Data["gateway"]).To(Equal("{{ (index .NetworkInterfaces ).Gateway }}"))
			Expect(updateArgs.VMMetadata.Data["nameserver"]).To(Equal(nameserver))
		})
	})
})
