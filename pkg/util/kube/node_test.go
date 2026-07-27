// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("GetESXHostInfoForNode", func() {
	const (
		nodeName = "node-1.example.com"
		hostMoID = "host-42"
		zoneName = "zone-a"
	)

	var (
		ctx    context.Context
		client ctrlclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = builder.NewFakeClient()
	})

	When("the Node does not exist", func() {
		It("returns an error", func() {
			_, _, err := kubeutil.GetESXHostInfoForNode(ctx, client, nodeName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(nodeName))
		})
	})

	When("the Node exists", func() {
		When("the Node has the host MOID annotation and zone label", func() {
			BeforeEach(func() {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:        nodeName,
						Annotations: map[string]string{"vmware-system-esxi-node-moid": hostMoID},
						Labels:      map[string]string{corev1.LabelTopologyZone: zoneName},
					},
				}
				Expect(client.Create(ctx, node)).To(Succeed())
			})

			It("returns the host MOID and zone", func() {
				gotHostMoID, gotZoneName, err := kubeutil.GetESXHostInfoForNode(ctx, client, nodeName)
				Expect(err).ToNot(HaveOccurred())
				Expect(gotHostMoID).To(Equal(hostMoID))
				Expect(gotZoneName).To(Equal(zoneName))
			})
		})

		When("the Node has no zone label", func() {
			BeforeEach(func() {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name:        nodeName,
						Annotations: map[string]string{"vmware-system-esxi-node-moid": hostMoID},
					},
				}
				Expect(client.Create(ctx, node)).To(Succeed())
			})

			It("returns the host MOID and an empty zone", func() {
				gotHostMoID, gotZoneName, err := kubeutil.GetESXHostInfoForNode(ctx, client, nodeName)
				Expect(err).ToNot(HaveOccurred())
				Expect(gotHostMoID).To(Equal(hostMoID))
				Expect(gotZoneName).To(BeEmpty())
			})
		})

		When("the Node does not have the host MOID annotation", func() {
			BeforeEach(func() {
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: nodeName},
				}
				Expect(client.Create(ctx, node)).To(Succeed())
			})

			It("returns an error", func() {
				_, _, err := kubeutil.GetESXHostInfoForNode(ctx, client, nodeName)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("vmware-system-esxi-node-moid"))
			})
		})
	})
})
