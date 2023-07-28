// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package simplelb

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

type cpArgs struct {
	service   *corev1.Service
	endpoints *corev1.Endpoints
}

type fakeControlPlane struct {
	calls []cpArgs
}

func (cp *fakeControlPlane) UpdateEndpoints(service *corev1.Service, endpoints *corev1.Endpoints) error {
	cp.calls = append(cp.calls, cpArgs{
		service:   service,
		endpoints: endpoints,
	})
	return nil
}

var _ = Describe("", func() {
	const (
		testNs  = "test-ns"
		testSvc = "test-svc"
		lbVMIP  = "11.12.13.14"
	)
	vmService := &vmopv1.VirtualMachineService{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testSvc,
		},
		Spec: vmopv1.VirtualMachineServiceSpec{
			Ports: []vmopv1.VirtualMachineServicePort{{
				Name:       "apiserver",
				Port:       6443,
				Protocol:   "TCP",
				TargetPort: 6443,
			}},
		},
	}
	vmKey := types.NamespacedName{Namespace: testNs, Name: testSvc + "-lb"}
	vm := &vmopv1.VirtualMachine{}
	client := builder.NewFakeClient(vmService)
	controlPlane := &fakeControlPlane{}
	simpleLbProvider := Provider{
		client:       client,
		controlPlane: controlPlane,
		log:          logr.Discard(),
	}
	vmImage := &vmopv1.VirtualMachineImage{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "loadbalancer-vm-",
		},
	}

	vmClass := &vmopv1.VirtualMachineClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "best-effort-small",
			Namespace: testNs,
		},
	}

	BeforeEach(func() {
		// ensure vm class and vm class binding exists.
		classList := &vmopv1.VirtualMachineClassList{}
		Expect(client.List(context.TODO(), classList)).To(Succeed())
		if len(classList.Items) == 0 {
			Expect(client.Create(context.TODO(), vmClass)).To(Succeed())
		}

		// ensure vm image exists.
		imageList := &vmopv1.VirtualMachineImageList{}
		Expect(client.List(context.TODO(), imageList)).To(Succeed())
		if len(imageList.Items) == 0 {
			Expect(client.Create(context.TODO(), vmImage)).To(Succeed())
		}
	})

	Context("EnsureLoadBalancer()", func() {
		It("should create the LB VM", func() {
			err := simpleLbProvider.EnsureLoadBalancer(context.TODO(), vmService)
			Expect(err).To(HaveOccurred())

			Expect(err.Error()).To(Equal("LB VM IP is not ready yet"))
			Expect(controlPlane.calls).To(BeEmpty())

			err = client.Get(context.TODO(), vmKey, vm)
			Expect(err).ToNot(HaveOccurred())
		})

		When("the LB VM has an IP address", func() {
			It("should update the VMService Loadbalancer IP", func() {
				vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{PrimaryIP4: lbVMIP}
				err := client.Status().Update(context.TODO(), vm)
				Expect(err).ToNot(HaveOccurred())

				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testSvc,
						Namespace: testNs,
					},
				}
				Expect(client.Create(context.TODO(), service)).To(Succeed())

				err = simpleLbProvider.EnsureLoadBalancer(context.TODO(), vmService)
				Expect(err).ToNot(HaveOccurred())

				Expect(controlPlane.calls).To(BeEmpty())

				err = client.Get(context.TODO(), types.NamespacedName{Namespace: testNs, Name: testSvc}, service)
				Expect(err).ToNot(HaveOccurred())
				Expect(service.Status.LoadBalancer.Ingress).To(HaveLen(1))
				Expect(service.Status.LoadBalancer.Ingress[0].IP).To(Equal(lbVMIP))
			})
		})

		When("Service and Endpoints have been created for VMService", func() {
			const (
				port     = 6443
				portName = "apiserver"
				ip1      = "10.11.12.13"
				ip2      = "21.22.23.24"
			)
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
					Namespace: testNs,
					Name:      testSvc,
				},
				Subsets: []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{IP: ip1}, {IP: ip2}},
					Ports: []corev1.EndpointPort{{
						Name: portName,
						Port: port,
					}},
				}},
			}
			It("should update the LB control plane", func() {
				err := client.Create(context.TODO(), eps)
				Expect(err).ToNot(HaveOccurred())

				err = simpleLbProvider.EnsureLoadBalancer(context.TODO(), vmService)
				Expect(err).ToNot(HaveOccurred())

				Expect(controlPlane.calls).To(HaveLen(1))
				Expect(controlPlane.calls[0].service.Name).To(Equal(svc.Name))
				Expect(controlPlane.calls[0].endpoints.Name).To(Equal(eps.Name))
			})
		})
	})
})
