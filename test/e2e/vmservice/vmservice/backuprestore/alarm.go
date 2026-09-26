// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backuprestore

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/alarm"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	"github.com/vmware-tanzu/vm-operator/test/e2e/vmservice/vmservice"
)

const (
	// registerVMAlarmName is the vCenter alarm raised by a failed RegisterVM
	// and cleared by a successful one. It is predefined in vCenter 9.0 and
	// later. On an older vCenter, create it with:
	//
	//	govc alarm.create -n WCPRegisterVMFailedAlarm \
	//	  -d "registervm failed (for e2e)" \
	//	  -green com.vmware.wcp.RegisterVM.success \
	//	  -yellow com.vmware.wcp.RegisterVM.failure
	registerVMAlarmName = "WCPRegisterVMFailedAlarm"

	registerVMEventPrefix  = "com.vmware.wcp.RegisterVM."
	registerVMEventSuccess = registerVMEventPrefix + "success"
	registerVMEventFailure = registerVMEventPrefix + "failure"
)

// findRegisterVMAlarm returns the RegisterVM alarm definition, or nil if the
// vCenter does not define it.
func findRegisterVMAlarm(ctx context.Context, c *vim25.Client) *mo.Alarm {
	alarms, err := alarm.NewManager(c).GetAlarm(ctx, c.ServiceContent.RootFolder)
	Expect(err).ToNot(HaveOccurred())

	for i := range alarms {
		if isRegisterVMAlarm(alarms[i].Info) {
			return &alarms[i]
		}
	}

	return nil
}

// isRegisterVMAlarm matches both the predefined alarm, whose system name
// carries the reserved "alarm." prefix, and a manually created one.
func isRegisterVMAlarm(info types.AlarmInfo) bool {
	return info.SystemName == "alarm."+registerVMAlarmName || info.Name == registerVMAlarmName
}

// verifyRegisterVMAlarm registers a VM that Veeam restored as a new VM twice:
// first with its stored VM resource made invalid, which must fail and raise
// the alarm, and then with the original resource, which must succeed and
// clear the alarm.
func verifyRegisterVMAlarm(
	ctx context.Context,
	t *testEnv,
	c *vim25.Client,
	wcpAlarm *mo.Alarm,
	vmName, moID string,
	diskCount int) {

	ns := t.input.WCPNamespaceName
	vmRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: moID}

	resourceYAML := vmResourceYAML(ctx, c, vmRef)

	collector, err := event.NewManager(c).CreateCollectorForEvents(ctx, types.EventFilterSpec{
		EventTypeId: []string{registerVMEventSuccess, registerVMEventFailure},
		Entity: &types.EventFilterSpecByEntity{
			Entity:    vmRef,
			Recursion: types.EventFilterSpecRecursionOptionSelf,
		},
	})
	Expect(err).ToNot(HaveOccurred())

	defer func() { _ = collector.Destroy(ctx) }()

	By("Verify the restored VM has no RegisterVM events or alarm yet")
	Expect(newRegisterVMEvents(ctx, collector, wcpAlarm)).To(BeEmpty())
	Expect(triggeredRegisterVMAlarm(ctx, c, vmRef)).To(BeNil())

	By("Make the restored VM resource invalid so RegisterVM fails")
	setVMResourceYAML(ctx, c, vmRef, "invalid-yaml")

	taskInfo, err := vmservice.InvokeRegisterVM(ctx, moID, ns, t.clusterProxy, t.input.WCPClient)
	Expect(err).To(HaveOccurred())
	Expect(taskInfo).ToNot(BeNil())
	Expect(taskInfo.Error).ToNot(BeNil())
	Expect(taskInfo.State).To(Equal(types.TaskInfoStateError))

	By("Verify the failure event was posted and raised the alarm")

	var events map[string][]types.EventEx

	Eventually(func(g Gomega) {
		events = newRegisterVMEvents(ctx, collector, wcpAlarm)
		g.Expect(events).To(HaveLen(1))
		g.Expect(events[registerVMEventFailure]).To(HaveLen(1))
	}, t.config.GetIntervals("default", "wait-config-map-creation")...).Should(Succeed(), "timed out waiting for the RegisterVM failure event")

	state := triggeredRegisterVMAlarm(ctx, c, vmRef)
	Expect(state).ToNot(BeNil())
	Expect(state.OverallStatus).To(Equal(types.ManagedEntityStatusYellow))
	Expect(state.Event).ToNot(BeNil())
	Expect(state.Event.(*types.EventEx).EventTypeId).To(Equal(registerVMEventFailure))
	Expect(state.EventKey).To(Equal(events[registerVMEventFailure][0].Key))

	By("Restore the original VM resource and register the VM again")
	setVMResourceYAML(ctx, c, vmRef, resourceYAML)
	t.registerVM(ctx, moID)

	vmservice.VerifyPostRegisterVM(ctx, vmName, ns, nil, diskCount, t.clusterProxy, t.config, t.client, t.input.WCPClient)

	By("Verify the success event was posted and cleared the alarm")
	Eventually(func(g Gomega) {
		events = newRegisterVMEvents(ctx, collector, wcpAlarm)
		g.Expect(events).To(HaveLen(1))
		g.Expect(events[registerVMEventSuccess]).To(HaveLen(1))
	}, t.config.GetIntervals("default", "wait-config-map-creation")...).Should(Succeed(), "timed out waiting for the RegisterVM success event")

	Expect(triggeredRegisterVMAlarm(ctx, c, vmRef)).To(BeNil())
}

// newRegisterVMEvents reads the events the collector has not returned yet,
// grouped by event type.
func newRegisterVMEvents(ctx context.Context, collector *event.HistoryCollector, wcpAlarm *mo.Alarm) map[string][]types.EventEx {
	events := map[string][]types.EventEx{}

	for {
		page, err := collector.ReadNextEvents(ctx, 10)
		Expect(err).ToNot(HaveOccurred())

		if len(page) == 0 {
			return events
		}

		for i := range page {
			e := page[i].(*types.EventEx)
			events[e.EventTypeId] = append(events[e.EventTypeId], *e)

			// RegisterVM posts the message itself; vCenter formats the full
			// message only for predefined alarms.
			Expect(e.Message).ToNot(BeEmpty())
			Expect(e.EventTypeId).To(HavePrefix(registerVMEventPrefix))

			if wcpAlarm.Info.SystemName != "" {
				Expect(e.FullFormattedMessage).ToNot(BeEmpty())
			}
		}
	}
}

// triggeredRegisterVMAlarm returns the VM's triggered RegisterVM alarm, or
// nil if it is not triggered.
func triggeredRegisterVMAlarm(ctx context.Context, c *vim25.Client, vmRef types.ManagedObjectReference) *alarm.StateInfo {
	states, err := alarm.NewManager(c).GetStateInfo(ctx, vmRef, alarm.StateInfoOptions{Event: true})
	Expect(err).ToNot(HaveOccurred())

	for i := range states {
		if isRegisterVMAlarm(*states[i].Info) {
			return &states[i]
		}
	}

	return nil
}

// vmResourceYAML returns the VM resource VM Operator stored in the VM's
// ExtraConfig, which Veeam restored along with the VM.
func vmResourceYAML(ctx context.Context, c *vim25.Client, vmRef types.ManagedObjectReference) string {
	var vmMO mo.VirtualMachine
	Expect(property.DefaultCollector(c).RetrieveOne(ctx, vmRef, []string{"config.extraConfig"}, &vmMO)).To(Succeed())
	Expect(vmMO.Config).ToNot(BeNil())

	value, _ := object.OptionValueList(vmMO.Config.ExtraConfig).GetString(backupapi.VMResourceYAMLExtraConfigKey)
	Expect(value).ToNot(BeEmpty(), "restored VM has no %s ExtraConfig", backupapi.VMResourceYAMLExtraConfigKey)

	return value
}

func setVMResourceYAML(ctx context.Context, c *vim25.Client, vmRef types.ManagedObjectReference, value string) {
	task, err := object.NewVirtualMachine(c, vmRef).Reconfigure(ctx, types.VirtualMachineConfigSpec{
		ExtraConfig: []types.BaseOptionValue{
			&types.OptionValue{Key: backupapi.VMResourceYAMLExtraConfigKey, Value: value},
		},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(task.Wait(ctx)).To(Succeed())
}
