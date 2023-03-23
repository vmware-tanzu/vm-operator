// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"encoding/base64"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

var _ = Describe("lbCloudConfig", func() {
	Context("envoyBootstrapConfigTemplate", func() {
		It("should be initialized", func() {
			Expect(envoyBootstrapConfigTemplate).ToNot(BeNil())
		})

		When("given properly initialized lbConfigParams", func() {
			vmService := &vmopv1.VirtualMachineService{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "testnamespace",
					Name:      "testname",
				},
				Spec: vmopv1.VirtualMachineServiceSpec{
					Ports: []vmopv1.VirtualMachineServicePort{{
						Name:       "apiserver",
						Protocol:   "TCP",
						Port:       6443,
						TargetPort: 6443,
					}},
				},
			}

			params := lbConfigParams{
				NodeID:      vmService.NamespacedName(),
				Ports:       vmService.Spec.Ports,
				CPNodes:     []string{"10.10.00.3"},
				XdsNodePort: XdsNodePort,
			}
			sb := &strings.Builder{}
			err := envoyBootstrapConfigTemplate.Execute(sb, params)
			s := sb.String()

			It("should render without an error", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(s).To(ContainSubstring(vmService.NamespacedName()))
				Expect(s).ToNot(ContainSubstring("\t"))
			})

			Context("renderAndBase64EncodeLBCloudConfig()", func() {
				It("should encode into valid base64 cloud-config", func() {
					b64s := renderAndBase64EncodeLBCloudConfig(params)

					ccBytes, err := base64.StdEncoding.DecodeString(b64s)
					Expect(err).NotTo(HaveOccurred())
					ccString := string(ccBytes)
					Expect(ccString).To(HavePrefix(cloudConfigPrefix))

					cc := cloudConfig{}
					err = yaml.Unmarshal(ccBytes, &cc)
					Expect(err).NotTo(HaveOccurred())
					Expect(cc.WriteFiles).To(HaveLen(1))
					Expect(cc.WriteFiles[0].Path).To(Equal("/etc/envoy/envoy.yaml"))
					Expect(cc.WriteFiles[0].Content).To(Equal(s))
				})
			})
		})
	})
})
