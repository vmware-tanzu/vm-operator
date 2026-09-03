// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/history"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	maxPageSize = 1000
)

// VCTask is a helper struct to inspect and compare vCenter Tasks.
type VCTask struct {
	TaskMoid    string
	Name        string
	Description string
	EntityMoid  string
	State       types.TaskInfoState
	Progress    int32
	ErrorDesc   string
}

// RecentTasks returns a list of vCenter Tasks fetched using the given Client. The pastDuration argument specifies
// how far back in time to gather Tasks, and any Tasks with older start times will be excluded. The entity argument
// allows optional filtering based on ManagedObjects in the vCenter inventory.
func RecentTasks(client *vim25.Client, pastDuration time.Duration, entity *types.ManagedObjectReference) []VCTask {
	now := getCurrentTime(client)
	recentTimeDuration := now.Add(-1 * pastDuration)

	return getTasksWithinTimeRange(client, entity, recentTimeDuration, time.Now())
}

// LookupTask checks vCenter's Recent Tasks for a match with the given name and description. An optional entity
// parameter supports restricting the Task list to a specific ManagedObject. Returns nil if there are no matches.
// Restricts search to Tasks that have started in the duration between pastDuration and now.
func LookupTask(client *vim25.Client, taskName, taskDescription string, pastDuration time.Duration,
	entity *types.ManagedObjectReference) *VCTask {
	recentTasks := RecentTasks(client, pastDuration, entity)
	framework.Logf("Searching %d recent Tasks for %s with description: %s", len(recentTasks), taskName,
		taskDescription)

	for _, task := range recentTasks {
		if task.Name == taskName && task.Description == taskDescription {
			return &task
		}
	}

	var taskStr strings.Builder
	for _, t := range recentTasks {
		fmt.Fprintf(&taskStr, "{Name: %s, Description: %s} \n", t.Name, t.Description)
	}

	framework.Logf("No tasks matching description: %s All recent Tasks: %s", taskDescription, taskStr.String())

	return nil
}

// WaitForTaskToBeComplete looks up a vCenter Task and waits on it to complete.
func WaitForTaskToBeComplete(client *vim25.Client, targetTask *VCTask) types.TaskInfoState {
	Expect(client).NotTo(BeNil())
	Expect(targetTask).NotTo(BeNil())

	// Construct a ManagedObjectReference for the Task and wait for it to reach terminal state.
	obj := types.ManagedObjectReference{
		Type:  "Task",
		Value: targetTask.TaskMoid,
	}
	taskObj := object.NewTask(client, obj)
	taskInfo, err := taskObj.WaitForResult(context.Background())
	Expect(err).NotTo(HaveOccurred())
	Expect(taskInfo).NotTo(BeNil())

	return taskInfo.State
}

// ExpectTaskToSucceed waits on a vCenter Task to complete and verifies that it succeeds.
func ExpectTaskToSucceed(client *vim25.Client, targetTask *VCTask) {
	taskState := WaitForTaskToBeComplete(client, targetTask)
	Expect(taskState).To(Equal(types.TaskInfoStateSuccess))
}

// WaitForNoActiveTask waits until no VM in vCenter inventory has a
// currently-running task matching descriptionID (e.g.
// "VirtualMachine.promoteDisks"), matched on DescriptionId -- not Name or
// the localized Description message, neither of which reliably identifies
// a task type.
//
// It reads each VM's recentTask property and resolves those refs' Info,
// the same mechanism vm-operator's reconciler uses (see getRecentTaskInfo
// in pkg/providers/vsphere/vmprovider_vm.go). vCenter's TaskHistoryCollector
// API doesn't reliably surface tasks already running before the collector
// was created, so it isn't used here.
//
// taskLabel is only for the By() step text. intervals is passed to
// Eventually (typically config.GetIntervals).
//
// Returns whether the task drained within intervals; it does not fail the
// spec itself -- callers decide what a non-drain means (e.g. Skip). This is
// a point-in-time check, not a lock: it can't prevent a matching task from
// starting right after it reports drained.
func WaitForNoActiveTask(
	ctx context.Context,
	client *vim25.Client,
	descriptionID, taskLabel string,
	intervals ...any) bool {

	By(fmt.Sprintf("Waiting for any in-progress %s tasks to finish", taskLabel))

	// Use a local Gomega with a fail handler that just records the outcome
	// instead of failing the spec, so a drain that never completes (timeout,
	// or persistent query errors) is reported back to the caller instead.
	drained := true
	localG := NewGomega(func(_ string, _ ...int) {
		drained = false
	})

	localG.Eventually(func(g Gomega) []string {
		running, err := runningTasksMatching(ctx, client, descriptionID)
		g.Expect(err).ToNot(HaveOccurred())
		return running
	}, intervals...).Should(BeEmpty())

	return drained
}

// runningTasksMatching returns the moref value of every currently-running
// task, across every VM in vCenter inventory, whose DescriptionId equals
// descriptionID. It does this in two property-collector round trips
// regardless of inventory size: one ContainerView.Retrieve to read every
// VM's recentTask in a single batched call, and one property.Collector
// Retrieve to resolve every one of those task refs' Info in a second
// batched call -- not one call per VM.
func runningTasksMatching(
	ctx context.Context,
	client *vim25.Client,
	descriptionID string) ([]string, error) {

	v, err := view.NewManager(client).CreateContainerView(
		ctx, client.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = v.Destroy(ctx) }()

	var vms []mo.VirtualMachine
	if err := v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"recentTask"}, &vms); err != nil {
		return nil, err
	}

	var taskRefs []types.ManagedObjectReference
	for _, vm := range vms {
		taskRefs = append(taskRefs, vm.RecentTask...)
	}
	if len(taskRefs) == 0 {
		return nil, nil
	}

	var tasks []mo.Task
	if err := property.DefaultCollector(client).Retrieve(
		ctx, taskRefs, []string{"info"}, &tasks); err != nil {

		return nil, err
	}

	var running []string
	for _, t := range tasks {
		if t.Info.State == types.TaskInfoStateRunning && t.Info.DescriptionId == descriptionID {
			running = append(running, t.Info.Task.Value)
		}
	}
	return running, nil
}

// getCurrentTime fetches the currentTime from the vCenter Server.
func getCurrentTime(client soap.RoundTripper) *time.Time {
	res, err := methods.GetCurrentTime(context.Background(), client)
	Expect(err).NotTo(HaveOccurred())

	return res
}

// getTasksWithinTimeRange uses a TaskHistoryCollector with Filters to narrow the search for recent Tasks.
func getTasksWithinTimeRange(client *vim25.Client, watch *types.ManagedObjectReference, start time.Time, end time.Time) []VCTask {
	Expect(client).NotTo(BeNil())

	// Setup Time filter based on start and end times.
	filter := types.TaskFilterSpec{
		Time: &types.TaskFilterSpecByTime{
			TimeType:  types.TaskFilterSpecTimeOptionStartedTime,
			BeginTime: &start,
			EndTime:   &end,
		},
	}

	// Add Entity filter if watch argument is specified.
	if watch != nil {
		filter.Entity = &types.TaskFilterSpecByEntity{
			Entity:    *watch,
			Recursion: types.TaskFilterSpecRecursionOptionSelf,
		}
	}

	taskInfo, err := fetchTaskInfoPage(context.Background(), client, filter)
	Expect(err).NotTo(HaveOccurred())

	tasks := []VCTask{}

	for _, task := range taskInfo {
		name := strings.TrimSuffix(task.Name, "_Task")
		if task.Entity == nil {
			continue
		}

		if len(name) == 0 {
			name = task.DescriptionId
		}

		description := task.DescriptionId
		if task.Description != nil {
			description = task.Description.Message
		}

		var taskErr string
		if task.Error != nil {
			taskErr = task.Error.LocalizedMessage
		}

		tasks = append(tasks, VCTask{
			TaskMoid:    task.Task.Value,
			Name:        name,
			Description: description,
			EntityMoid:  task.Entity.Value,
			State:       task.State,
			Progress:    task.Progress,
			ErrorDesc:   taskErr,
		})
	}

	return tasks
}

// fetchTaskInfoPage creates a TaskHistoryCollector for the given filter,
// resets it to the latest matching tasks, fetches that page, and destroys
// the collector. This is the shared plumbing behind every task-query entry
// point in this file (time-windowed history for RecentTasks/LookupTask,
// state-only for WaitForNoActiveTask); callers apply their own
// filtering/mapping semantics on top of the raw TaskInfo it returns.
func fetchTaskInfoPage(
	ctx context.Context,
	client *vim25.Client,
	filter types.TaskFilterSpec) ([]types.TaskInfo, error) {

	taskReq := types.CreateCollectorForTasks{
		This:   *client.ServiceContent.TaskManager,
		Filter: filter,
	}
	res, err := methods.CreateCollectorForTasks(ctx, client, &taskReq)
	if err != nil {
		return nil, err
	}

	collector := history.NewCollector(client, res.Returnval)
	defer func() {
		if err := collector.Destroy(ctx); err != nil {
			// Collectors should be cleaned up but an error here is not fatal.
			framework.Logf("Failed to Destroy TaskHistoryCollector: %v", err)
		}
	}()

	if err := collector.Reset(ctx); err != nil {
		return nil, err
	}

	if err := collector.SetPageSize(ctx, maxPageSize); err != nil {
		return nil, err
	}

	var page mo.TaskHistoryCollector
	if err := collector.Properties(
		ctx, collector.Reference(), []string{"latestPage"}, &page); err != nil {

		return nil, err
	}

	return page.LatestPage, nil
}
