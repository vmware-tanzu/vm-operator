// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"context"
	"errors"
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/diskpromo"
)

// vmRecentTasksTests uses promotion tasks to verify ordering through the provider:
// disk promotion sets its condition from the first success or non-transient error
// task, so reversing those tasks changes the observable condition.
func vmRecentTasksTests() {
	DescribeTable("retrieves available tasks while tolerating expired references",
		func(states []vimtypes.TaskInfoState, options recentTaskTestOptions) {
			vmClass, vm := newVMTestObjects("test-vm")
			ctx, provider, _ := setupVMTest(
				newVMTestParentContext(), newVMTestConfig(), vmClass, vm)
			defer vmTestAfterEach(ctx, vm)

			vcVM, err := createOrUpdateAndGetVcVM(ctx, provider, vm)
			Expect(err).ToNot(HaveOccurred())
			simCtx := ctx.SimulatorContext()
			reg := simCtx.Map
			var refs []vimtypes.ManagedObjectReference
			if options.expiredReferences {
				refs = append(refs, vimtypes.ManagedObjectReference{
					Type: "Task", Value: "task-expired-before",
				})
			}
			for i, state := range states {
				task := &mo.Task{Info: vimtypes.TaskInfo{
					Key:           fmt.Sprintf("recent-task-%d", i),
					State:         state,
					DescriptionId: "fake.task",
				}}
				if options.promotionTasks {
					task.Info.DescriptionId = diskpromo.PromoteDisksTaskKey
				}
				if state == vimtypes.TaskInfoStateError {
					task.Info.Error = &vimtypes.LocalizedMethodFault{
						Fault: &vimtypes.InvalidState{}, LocalizedMessage: "promotion failed",
					}
				}
				ref := reg.Put(task).Reference()
				task.Info.Task = ref
				defer reg.Remove(simCtx, ref)
				refs = append(refs, ref)
			}
			if options.expiredReferences {
				refs = append(refs, vimtypes.ManagedObjectReference{
					Type: "Task", Value: "task-expired-after",
				})
			}
			reg.WithLock(simCtx, vcVM.Reference(), func() {
				simVM, ok := reg.Get(vcVM.Reference()).(*simulator.VirtualMachine)
				Expect(ok).To(BeTrue())
				simVM.RecentTask = refs
			})

			client, err := provider.VSphereClient(ctx)
			Expect(err).ToNot(HaveOccurred())
			vimClient := client.VimClient()
			original := vimClient.RoundTripper
			reordered := &recentTasksRoundTripper{RoundTripper: original}
			vimClient.RoundTripper = reordered
			defer func() { vimClient.RoundTripper = original }()

			err = createOrUpdateVM(ctx, provider, vm)
			if options.runningTask {
				Expect(errors.Is(err, vsphere.ErrHasTask)).To(BeTrue())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			if len(refs) == 0 {
				Expect(reordered.requests).To(BeEmpty())
			} else {
				Expect(reordered.requests).ToNot(BeEmpty())
				for _, requested := range reordered.requests {
					Expect(requested).To(Equal(refs))
				}
			}
			if options.promotionTasks {
				condition := conditions.Get(vm, vmopv1.VirtualMachineDiskPromotionSynced)
				Expect(condition).ToNot(BeNil())
				if states[0] == vimtypes.TaskInfoStateError {
					Expect(condition.Reason).To(Equal(diskpromo.ReasonTaskError))
				} else {
					Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				}
			}
		},
		Entry("no tasks", nil, recentTaskTestOptions{}),
		Entry("all tasks expired", nil, recentTaskTestOptions{expiredReferences: true}),
		Entry("multiple completed tasks", []vimtypes.TaskInfoState{
			vimtypes.TaskInfoStateSuccess, vimtypes.TaskInfoStateSuccess,
		}, recentTaskTestOptions{}),
		Entry("running task after completed tasks", []vimtypes.TaskInfoState{
			vimtypes.TaskInfoStateSuccess, vimtypes.TaskInfoStateRunning,
		}, recentTaskTestOptions{runningTask: true}),
		Entry("expired references surrounding a running task", []vimtypes.TaskInfoState{
			vimtypes.TaskInfoStateSuccess, vimtypes.TaskInfoStateRunning,
		}, recentTaskTestOptions{expiredReferences: true, runningTask: true}),
		Entry("expired references surrounding completed tasks", []vimtypes.TaskInfoState{
			vimtypes.TaskInfoStateSuccess, vimtypes.TaskInfoStateSuccess,
		}, recentTaskTestOptions{expiredReferences: true}),
		Entry("promotion success before error preserves the first result", []vimtypes.TaskInfoState{
			vimtypes.TaskInfoStateSuccess, vimtypes.TaskInfoStateError,
		}, recentTaskTestOptions{promotionTasks: true}),
		Entry("promotion error before success preserves the first result", []vimtypes.TaskInfoState{
			vimtypes.TaskInfoStateError, vimtypes.TaskInfoStateSuccess,
		}, recentTaskTestOptions{promotionTasks: true, expiredReferences: true}),
	)
}

type recentTaskTestOptions struct {
	promotionTasks    bool
	expiredReferences bool
	runningTask       bool
}

// recentTasksRoundTripper reverses batch results to exercise vCenter ordering.
type recentTasksRoundTripper struct {
	soap.RoundTripper
	requests [][]vimtypes.ManagedObjectReference
}

func (r *recentTasksRoundTripper) RoundTrip(ctx context.Context, req, res soap.HasFault) error {
	if err := r.RoundTripper.RoundTrip(ctx, req, res); err != nil {
		return err
	}
	request, ok := req.(*methods.RetrievePropertiesExBody)
	if !ok || request.Req == nil {
		return nil
	}
	taskRequest := false
	for _, spec := range request.Req.SpecSet {
		for _, prop := range spec.PropSet {
			taskRequest = taskRequest || prop.Type == "Task"
		}
	}
	if !taskRequest {
		return nil
	}
	if body, ok := res.(*methods.RetrievePropertiesExBody); ok && body.Res != nil {
		var refs []vimtypes.ManagedObjectReference
		for _, spec := range request.Req.SpecSet {
			for _, object := range spec.ObjectSet {
				refs = append(refs, object.Obj)
			}
		}
		r.requests = append(r.requests, refs)
		if body.Res.Returnval != nil {
			slices.Reverse(body.Res.Returnval.Objects)
		}
	}
	return nil
}
