// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/keyboard"
)

var _ keyboard.ScanCodeSender = &fakeScanCodeSender{}

type fakeScanCodeSender struct {
	calls int
	err   error
}

func (f *fakeScanCodeSender) PutUsbScanCodes(
	_ context.Context, spec vimtypes.UsbScanCodeSpec) (int32, error) {

	if f.err != nil {
		return 0, f.err
	}
	f.calls++
	return int32(len(spec.KeyEvents)), nil
}

var _ = Describe("BootstrapISO", func() {
	const (
		vmName    = "my-vm-1"
		vmNS      = "my-namespace-1"
		svcName   = vmName + "-iso-bootstrap"
		serviceIP = "10.0.0.5"
	)

	var (
		k8sClient ctrlclient.Client
		sender    *fakeScanCodeSender
		vm        *vmopv1.VirtualMachine
		vmCtx     pkgctx.VirtualMachineContext
		bsArgs    *vmlifecycle.BootstrapArgs
		scheme    *runtime.Scheme
	)

	newReadyService := func() *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: vmNS},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{{IP: serviceIP}},
				},
			},
		}
	}

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(vmopv1.AddToScheme(scheme)).To(Succeed())

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: vmNS},
			Spec: vmopv1.VirtualMachineSpec{
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					ISO: &vmopv1.VirtualMachineBootstrapISOSpec{
						Commands: []string{"boot"},
					},
				},
			},
		}

		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm).Build()
		sender = &fakeScanCodeSender{}
		bsArgs = &vmlifecycle.BootstrapArgs{}

		vmCtx = pkgctx.NewVirtualMachineContext(pkgcfg.NewContextWithDefaultConfig(), vm)
	})

	When("the bootstrap-iso-synced condition is already true", func() {
		BeforeEach(func() {
			conditions.MarkTrue(vm, vmopv1.VirtualMachineBootstrapISOSynced)
		})

		It("returns nil without touching the HTTP server or keyboard", func() {
			err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
			Expect(err).ToNot(HaveOccurred())
			Expect(sender.calls).To(Equal(0))

			pods := &corev1.PodList{}
			Expect(k8sClient.List(vmCtx, pods)).To(Succeed())
			Expect(pods.Items).To(BeEmpty())
		})
	})

	When("a referenced asset does not exist", func() {
		BeforeEach(func() {
			vm.Spec.Bootstrap.ISO.Assets = []vmopv1.VirtualMachineBootstrapISOAsset{
				{Name: "does-not-exist", Key: "user-data"},
			}
		})

		It("marks the condition False with AssetNotFound and returns an error", func() {
			err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
			Expect(err).To(HaveOccurred())

			c := conditions.Get(vm, vmopv1.VirtualMachineBootstrapISOSynced)
			Expect(c).ToNot(BeNil())
			Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBootstrapISOAssetNotFoundReason))
		})
	})

	When("the HTTP server has no address yet", func() {
		It("requeues without sending any commands", func() {
			err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
			var requeueErr pkgerr.RequeueError
			Expect(errors.As(err, &requeueErr)).To(BeTrue())
			Expect(sender.calls).To(Equal(0))

			c := conditions.Get(vm, vmopv1.VirtualMachineBootstrapISOSynced)
			Expect(c).ToNot(BeNil())
			Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBootstrapISOHTTPServerNotReadyReason))
		})
	})

	When("the HTTP server has an address and commands have not been sent", func() {
		BeforeEach(func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, newReadyService()).Build()
		})

		It("sends the commands, stamps the hash annotation, and returns ErrBootstrapISO", func() {
			err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
			Expect(err).To(MatchError(vmlifecycle.ErrBootstrapISO))
			Expect(sender.calls).To(BeNumerically(">", 0))
			Expect(vm.Annotations[pkgconst.BootstrapISOHashAnnotationKey]).ToNot(BeEmpty())
			Expect(vm.Annotations[pkgconst.BootstrapISOStartedAtAnnotationKey]).ToNot(BeEmpty())

			c := conditions.Get(vm, vmopv1.VirtualMachineBootstrapISOSynced)
			Expect(c).ToNot(BeNil())
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal("CommandsSent"))
		})

		When("the total wait time exceeds the 120s cap", func() {
			BeforeEach(func() {
				vm.Spec.Bootstrap.ISO.Commands = []string{"<wait1m>", "<wait1m>", "<wait5s>"}
			})

			It("fails before sending anything", func() {
				err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
				Expect(err).To(HaveOccurred())
				Expect(sender.calls).To(Equal(0))
				Expect(vm.Annotations[pkgconst.BootstrapISOHashAnnotationKey]).To(BeEmpty())

				c := conditions.Get(vm, vmopv1.VirtualMachineBootstrapISOSynced)
				Expect(c).ToNot(BeNil())
				Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBootstrapISOKeyboardSendFailedReason))
			})
		})

		When("sending commands fails", func() {
			BeforeEach(func() {
				sender.err = errors.New("boom")
			})

			It("returns a wrapped error and does not stamp the hash annotation", func() {
				err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
				Expect(err).To(MatchError(ContainSubstring("boom")))
				Expect(vm.Annotations[pkgconst.BootstrapISOHashAnnotationKey]).To(BeEmpty())

				c := conditions.Get(vm, vmopv1.VirtualMachineBootstrapISOSynced)
				Expect(c).ToNot(BeNil())
				Expect(c.Reason).To(Equal(vmopv1.VirtualMachineBootstrapISOKeyboardSendFailedReason))
			})
		})
	})

	When("commands were already sent and installation is still in progress", func() {
		BeforeEach(func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, newReadyService()).Build()

			// First call sends commands and stamps the hash annotation.
			err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
			Expect(err).To(MatchError(vmlifecycle.ErrBootstrapISO))
			Expect(sender.calls).To(Equal(1))
		})

		It("requeues without resending commands or tearing down the HTTP server", func() {
			err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
			var requeueErr pkgerr.RequeueError
			Expect(errors.As(err, &requeueErr)).To(BeTrue())
			Expect(sender.calls).To(Equal(1), "commands must not be sent twice")

			pods := &corev1.PodList{}
			Expect(k8sClient.List(vmCtx, pods)).To(Succeed())
			Expect(pods.Items).To(HaveLen(1), "the http server must not be torn down yet")
		})

		When("the VM has since reported a primary IP", func() {
			BeforeEach(func() {
				vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{PrimaryIP4: "192.168.1.10"}
			})

			It("tears down the http server and marks the condition True", func() {
				err := vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)
				Expect(err).ToNot(HaveOccurred())
				Expect(sender.calls).To(Equal(1), "commands must not be resent")

				pods := &corev1.PodList{}
				Expect(k8sClient.List(vmCtx, pods)).To(Succeed())
				Expect(pods.Items).To(BeEmpty())

				svcs := &corev1.ServiceList{}
				Expect(k8sClient.List(vmCtx, svcs)).To(Succeed())
				Expect(svcs.Items).To(BeEmpty())

				Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineBootstrapISOSynced)).To(BeTrue())
			})
		})
	})

	When("BootstrapISO is called again after the condition is already True", func() {
		BeforeEach(func() {
			k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(vm, newReadyService()).Build()

			Expect(vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)).
				To(MatchError(vmlifecycle.ErrBootstrapISO))
			vm.Status.Network = &vmopv1.VirtualMachineNetworkStatus{PrimaryIP4: "192.168.1.10"}
			Expect(vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)).To(Succeed())
			Expect(conditions.IsTrue(vm, vmopv1.VirtualMachineBootstrapISOSynced)).To(BeTrue())
		})

		It("never recreates the http server", func() {
			Expect(vmlifecycle.BootstrapISO(vmCtx, sender, k8sClient, vm.Spec.Bootstrap.ISO, bsArgs)).To(Succeed())

			pods := &corev1.PodList{}
			Expect(k8sClient.List(vmCtx, pods)).To(Succeed())
			Expect(pods.Items).To(BeEmpty())
		})
	})
})
