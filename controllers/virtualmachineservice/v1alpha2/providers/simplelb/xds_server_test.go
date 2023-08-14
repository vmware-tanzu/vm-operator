// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("xdsServer", func() {
	const (
		testNs       = "test-ns"
		testSvc      = "test-svc"
		epResVersion = "123"
		port         = 6443
		portName     = "apiserver"
		ip1          = "10.11.12.13"
		ip2          = "21.22.23.24"
	)

	x := &XdsServer{
		snapshotCache: cache.NewSnapshotCache(false, cache.IDHash{}, nil),
		log:           logr.Discard(),
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testSvc,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:     portName,
				Protocol: "TCP",
				Port:     port,
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: port,
				},
			}},
		},
	}
	eps := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       testNs,
			Name:            testSvc,
			ResourceVersion: epResVersion,
		},
		Subsets: []corev1.EndpointSubset{{
			Addresses: []corev1.EndpointAddress{{IP: ip1}, {IP: ip2}},
			Ports: []corev1.EndpointPort{{
				Name: portName,
				Port: port,
			}},
		}},
	}

	It("UpdateEndpoints()", func() {
		err := x.UpdateEndpoints(svc, eps)
		Expect(err).ToNot(HaveOccurred())

		snapshot, err := x.snapshotCache.GetSnapshot(nodeID(svc))
		Expect(err).ToNot(HaveOccurred())

		err = snapshot.Consistent()
		Expect(err).ToNot(HaveOccurred())

		Expect(snapshot.GetVersion(cache.EndpointType)).To(Equal(epResVersion))
		Expect(snapshot.GetVersion(cache.ClusterType)).To(Equal(epResVersion))

		clusters := snapshot.GetResources(cache.ClusterType)
		endpoints := snapshot.GetResources(cache.EndpointType)
		Expect(clusters).To(HaveLen(1))
		Expect(endpoints).To(HaveLen(1))
		Expect(endpoints[portName]).ToNot(BeNil())
		Expect(endpoints[portName].String()).To(ContainSubstring(ip1))
		Expect(endpoints[portName].String()).To(ContainSubstring(ip2))
	})
})
