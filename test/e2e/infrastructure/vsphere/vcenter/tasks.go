// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/history"
	"github.com/vmware/govmomi/object"
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

	ctx := context.Background()
	taskReq := types.CreateCollectorForTasks{
		This:   *client.ServiceContent.TaskManager,
		Filter: filter,
	}
	res, err := methods.CreateCollectorForTasks(ctx, client, &taskReq)
	Expect(err).NotTo(HaveOccurred())

	collector := history.NewCollector(client, res.Returnval)
	err = collector.Reset(ctx)
	Expect(err).NotTo(HaveOccurred())

	err = collector.SetPageSize(ctx, maxPageSize)
	Expect(err).NotTo(HaveOccurred())

	var (
		taskInfo             []types.TaskInfo
		taskHistoryCollector mo.TaskHistoryCollector
	)

	err = collector.Properties(ctx, collector.Reference(), []string{"latestPage"}, &taskHistoryCollector)
	Expect(err).NotTo(HaveOccurred())

	taskInfo = append(taskInfo, taskHistoryCollector.LatestPage...)

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

	err = collector.Destroy(ctx)
	if err != nil {
		// Collectors should be cleaned up but an error here is not fatal.
		framework.Logf("Failed to Destroy TaskHistoryCollector: %v", err)
	}

	return tasks
}
