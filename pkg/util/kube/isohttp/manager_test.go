// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package isohttp_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/isohttp"
)

// ctrlClient is a local alias to keep var blocks below readable.
type ctrlClient = ctrlclient.Client

// objKey returns the deterministic Pod/Service object key for vm.
func objKey(vm *vmopv1.VirtualMachine) ctrlclient.ObjectKey {
	return ctrlclient.ObjectKey{Namespace: vm.Namespace, Name: vm.Name + "-iso-bootstrap"}
}

var _ = Describe("Manager", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
		vm     *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()

		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-vm-1",
				Namespace: "my-namespace-1",
				UID:       types.UID("vm-uid-1"),
			},
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					ISO: &vmopv1.VirtualMachineBootstrapISOSpec{},
				},
			},
		}
	})

	Describe("EnsureReady", func() {
		var (
			client ctrlClient
			mgr    *isohttp.Manager
		)

		BeforeEach(func() {
			client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
		})

		JustBeforeEach(func() {
			mgr = isohttp.NewManager(client, vm)
		})

		It("creates a deterministically-named Pod and Service", func() {
			_, _, ready, err := mgr.EnsureReady(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(ready).To(BeFalse())

			pod := &corev1.Pod{}
			Expect(client.Get(ctx, objKey(vm), pod)).To(Succeed())
			Expect(pod.Name).To(Equal("my-vm-1-iso-bootstrap"))
			Expect(pod.Labels[pkgconst.ISOBootstrapVMLabelKey]).To(Equal("my-vm-1"))
			Expect(pod.OwnerReferences).To(HaveLen(1))
			Expect(pod.OwnerReferences[0].UID).To(Equal(vm.UID))

			svc := &corev1.Service{}
			Expect(client.Get(ctx, objKey(vm), svc)).To(Succeed())
			Expect(svc.Name).To(Equal("my-vm-1-iso-bootstrap"))
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			Expect(svc.OwnerReferences).To(HaveLen(1))
		})

		It("is idempotent across repeated calls", func() {
			_, _, _, err := mgr.EnsureReady(ctx)
			Expect(err).ToNot(HaveOccurred())

			_, _, _, err = mgr.EnsureReady(ctx)
			Expect(err).ToNot(HaveOccurred())

			pods := &corev1.PodList{}
			Expect(client.List(ctx, pods)).To(Succeed())
			Expect(pods.Items).To(HaveLen(1))

			svcs := &corev1.ServiceList{}
			Expect(client.List(ctx, svcs)).To(Succeed())
			Expect(svcs.Items).To(HaveLen(1))
		})

		When("the Service has no LoadBalancer ingress yet", func() {
			It("reports not ready without an error", func() {
				address, port, ready, err := mgr.EnsureReady(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(ready).To(BeFalse())
				Expect(address).To(BeEmpty())
				Expect(port).To(Equal(0))
			})
		})

		When("the Service has a LoadBalancer ingress IP", func() {
			BeforeEach(func() {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-vm-1-iso-bootstrap",
						Namespace: vm.Namespace,
					},
				}
				client = fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(vm).
					WithStatusSubresource(svc).
					Build()
				Expect(client.Create(ctx, svc)).To(Succeed())
				svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "10.0.0.5"}}
				Expect(client.Status().Update(ctx, svc)).To(Succeed())
			})

			It("returns the ingress address and port", func() {
				address, port, ready, err := mgr.EnsureReady(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(ready).To(BeTrue())
				Expect(address).To(Equal("10.0.0.5"))
				Expect(port).To(Equal(80))
			})
		})

		When("spec.bootstrap.iso.assets references a Secret that does not exist", func() {
			BeforeEach(func() {
				vm.Spec.Bootstrap.ISO.Assets = []vmopv1.VirtualMachineBootstrapISOAsset{
					{Name: "my-vm-1-bootstrap-data", Key: "user-data"},
				}
			})

			It("returns ErrAssetNotFound and creates nothing", func() {
				_, _, _, err := mgr.EnsureReady(ctx)
				var notFound *isohttp.ErrAssetNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))

				pods := &corev1.PodList{}
				Expect(client.List(ctx, pods)).To(Succeed())
				Expect(pods.Items).To(BeEmpty())
			})
		})

		When("spec.bootstrap.iso.assets references a key that does not exist in the Secret", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-vm-1-bootstrap-data",
						Namespace: vm.Namespace,
					},
					Data: map[string][]byte{"meta-data": []byte("")},
				}
				client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, secret).Build()
				vm.Spec.Bootstrap.ISO.Assets = []vmopv1.VirtualMachineBootstrapISOAsset{
					{Name: "my-vm-1-bootstrap-data", Key: "user-data"},
				}
			})

			It("returns ErrAssetNotFound", func() {
				_, _, _, err := mgr.EnsureReady(ctx)
				var notFound *isohttp.ErrAssetNotFound
				Expect(err).To(BeAssignableToTypeOf(notFound))
			})
		})

		When("spec.bootstrap.iso.assets references an existing Secret and key", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-vm-1-bootstrap-data",
						Namespace: vm.Namespace,
					},
					Data: map[string][]byte{
						"user-data": []byte("#cloud-config"),
						"meta-data": []byte(""),
					},
				}
				client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, secret).Build()
				vm.Spec.Bootstrap.ISO.Assets = []vmopv1.VirtualMachineBootstrapISOAsset{
					{Name: "my-vm-1-bootstrap-data", Key: "user-data"},
					{Name: "my-vm-1-bootstrap-data", Key: "meta-data"},
				}
			})

			It("mounts the Secret once and creates the Pod", func() {
				_, _, _, err := mgr.EnsureReady(ctx)
				Expect(err).ToNot(HaveOccurred())

				pod := &corev1.Pod{}
				Expect(client.Get(ctx, objKey(vm), pod)).To(Succeed())
				Expect(pod.Spec.Volumes).To(HaveLen(1))
				Expect(pod.Spec.Volumes[0].Secret.SecretName).To(Equal("my-vm-1-bootstrap-data"))
				Expect(pod.Spec.Containers).To(HaveLen(1))
				Expect(pod.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
				Expect(pod.Spec.Containers[0].VolumeMounts[0].MountPath).To(
					Equal("/data/my-vm-1-bootstrap-data"))
			})
		})
	})

	Describe("Teardown", func() {
		var client ctrlClient

		BeforeEach(func() {
			client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
		})

		It("deletes the Pod and Service", func() {
			mgr := isohttp.NewManager(client, vm)
			_, _, _, err := mgr.EnsureReady(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(mgr.Teardown(ctx)).To(Succeed())

			pods := &corev1.PodList{}
			Expect(client.List(ctx, pods)).To(Succeed())
			Expect(pods.Items).To(BeEmpty())

			svcs := &corev1.ServiceList{}
			Expect(client.List(ctx, svcs)).To(Succeed())
			Expect(svcs.Items).To(BeEmpty())
		})

		It("is idempotent when nothing was ever created", func() {
			mgr := isohttp.NewManager(client, vm)
			Expect(mgr.Teardown(ctx)).To(Succeed())
			Expect(mgr.Teardown(ctx)).To(Succeed())
		})
	})
})
