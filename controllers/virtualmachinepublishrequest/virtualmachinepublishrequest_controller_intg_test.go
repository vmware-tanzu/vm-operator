// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinepublishrequest_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking VirtualMachinePublishRequest controller tests", virtualMachinePublishRequestReconcile)
}

func virtualMachinePublishRequestReconcile() {
	var (
		ctx   *builder.IntegrationTestContext
		vmpub *vmopv1alpha1.VirtualMachinePublishRequest
		vm    *vmopv1alpha1.VirtualMachine
		cl    *imgregv1a1.ContentLibrary
	)

	getVirtualMachinePublishRequest := func(ctx *builder.IntegrationTestContext, objKey client.ObjectKey) *vmopv1alpha1.VirtualMachinePublishRequest {
		vmpub := &vmopv1alpha1.VirtualMachinePublishRequest{}
		if err := ctx.Client.Get(ctx, objKey, vmpub); err != nil {
			return nil
		}
		return vmpub
	}

	BeforeEach(func() {
		ctx = suite.NewIntegrationTestContext()

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: ctx.Namespace,
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ImageName:  "dummy-image",
				ClassName:  "dummy-class",
				PowerState: vmopv1alpha1.VirtualMachinePoweredOn,
			},
		}

		vmpub = builder.DummyVirtualMachinePublishRequest("dummy-vmpub", ctx.Namespace, vm.Name,
			"dummy-item", "dummy-cl")
		cl = builder.DummyContentLibrary("dummy-cl", ctx.Namespace, "dummy-cl")
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("Reconcile", func() {
		var (
			itemID string
		)

		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, vm)).To(Succeed())
			vmObj := &vmopv1alpha1.VirtualMachine{}
			Expect(ctx.Client.Get(ctx, client.ObjectKeyFromObject(vm), vmObj)).To(Succeed())
			vmObj.Status = vmopv1alpha1.VirtualMachineStatus{
				Phase:    vmopv1alpha1.Created,
				UniqueID: "dummy-unique-id",
			}
			Expect(ctx.Client.Status().Update(ctx, vmObj)).To(Succeed())
			Expect(ctx.Client.Create(ctx, cl)).To(Succeed())

			Eventually(func() error {
				return ctx.Client.Create(ctx, vmpub)
			}).Should(Succeed())

			defaultRequeueDelay := int64(virtualmachinepublishrequest.DefaultRequeueDelaySeconds)
			atomic.StoreInt64(&defaultRequeueDelay, 1)
			virtualmachinepublishrequest.DefaultRequeueDelaySeconds = int(atomic.LoadInt64(&defaultRequeueDelay))

			itemID = uuid.New().String()
			intgFakeVMProvider.Lock()
			intgFakeVMProvider.PublishVirtualMachineFn = func(_ context.Context, vm *vmopv1alpha1.VirtualMachine,
				vmPub *vmopv1alpha1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
				intgFakeVMProvider.AddToVMPublishMap(actID, types.TaskInfoStateQueued)
				time.Sleep(1 * time.Second)
				intgFakeVMProvider.AddToVMPublishMap(actID, types.TaskInfoStateRunning)
				time.Sleep(1 * time.Second)
				intgFakeVMProvider.AddToVMPublishMap(actID, types.TaskInfoStateSuccess)

				return "dummy-id", nil
			}

			intgFakeVMProvider.GetTasksByActIDFn = func(_ context.Context, actID string) (tasksInfo []types.TaskInfo, retErr error) {
				state := intgFakeVMProvider.GetVMPublishRequestResultWithActIDLocked(actID)
				task := types.TaskInfo{
					DescriptionId: virtualmachinepublishrequest.TaskDescriptionID,
					State:         state,
				}

				if state == types.TaskInfoStateSuccess {
					task.Result = types.ManagedObjectReference{Type: "ContentLibraryItem", Value: fmt.Sprintf("clibitem-%s", itemID)}
				}

				return []types.TaskInfo{task}, nil
			}
			intgFakeVMProvider.Unlock()

			go func() {
				// create ContentLibraryItem and VirtualMachineImage once the task succeeds.
				for {
					obj := getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
					if state := intgFakeVMProvider.GetVMPublishRequestResult(obj); state == types.TaskInfoStateSuccess {
						clitem := &imgregv1a1.ContentLibraryItem{}
						err := ctx.Client.Get(ctx, client.ObjectKey{Name: "dummy-clitem", Namespace: ctx.Namespace}, clitem)
						if k8serrors.IsNotFound(err) {
							clitem = utils.DummyContentLibraryItem("dummy-clitem", ctx.Namespace)
							Expect(ctx.Client.Create(ctx, clitem)).To(Succeed())

							vmi := builder.DummyVirtualMachineImage("dummy-image")
							vmi.Namespace = ctx.Namespace
							vmi.Spec.ImageID = itemID
							Expect(ctx.Client.Create(ctx, vmi)).To(Succeed())
							vmi.Status.ImageName = vmpub.Spec.Target.Item.Name
							Expect(ctx.Client.Status().Update(ctx, vmi)).To(Succeed())

							return
						}
					}
					time.Sleep(time.Second)
				}
			}()
		})

		AfterEach(func() {
			err := ctx.Client.Delete(ctx, vmpub)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, vm)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())
			err = ctx.Client.Delete(ctx, cl)
			Expect(err == nil || k8serrors.IsNotFound(err)).To(BeTrue())

			intgFakeVMProvider.Reset()
		})

		It("VirtualMachinePublishRequest completed", func() {
			obj := &vmopv1alpha1.VirtualMachinePublishRequest{}
			Eventually(func() bool {
				obj = getVirtualMachinePublishRequest(ctx, client.ObjectKeyFromObject(vmpub))
				return obj != nil && obj.IsComplete()
			}).Should(BeTrue())

			Expect(obj.Status.Ready).To(BeTrue())
			Expect(obj.Status.ImageName).To(Equal("dummy-image"))
			Expect(obj.Status.CompletionTime).NotTo(BeZero())
		})
	})
}
