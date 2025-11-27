// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package cdrom_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/cdrom"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(cdrom.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'cdrom'", func() {
		Expect(cdrom.New().Name()).To(Equal("cdrom"))
	})
})

var _ = Describe("Reconcile", func() {

	var (
		r          vmconfig.Reconciler
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vm         *vmopv1.VirtualMachine
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		r = cdrom.New()

		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = vmconfig.WithContext(ctx)

		vimClient = vcsimCtx.VCClient.Client

		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	Context("a panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})
		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})
		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
		When("configSpec is nil", func() {
			JustBeforeEach(func() {
				configSpec = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("configSpec is nil"))
			})
		})
	})

	When("no panic is expected", func() {
		var (
			err error
		)

		JustBeforeEach(func() {
			err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
		})

		Context("basic reconciliation", func() {
			When("reconciling with empty VM spec", func() {
				It("should succeed without error", func() {
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})
})
