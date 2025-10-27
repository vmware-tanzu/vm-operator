# VirtualMachine Controller

The `VirtualMachine` controller is responsible for reconciling `VirtualMachine` objects.

## Priority

The VirtualMachine controller leverages controller-runtime's [priority queue feature](https://github.com/kubernetes-sigs/controller-runtime/issues/2374) to intelligently order reconciliation requests based on the operational state of each VM. Priority queues allow the controller to process more urgent operations (like VM creation or deletion) ahead of routine maintenance operations (like resync events), improving system responsiveness and resource utilization.

### Priority queues

Controller-runtime's priority queue implementation uses a min-heap data structure where items with **higher numerical values have higher priority** and are dequeued first. When a Kubernetes event (create, update, delete, or generic) triggers a reconciliation, the controller can assign a specific priority value to that request. This ensures that critical operations don't get stuck behind less urgent work, especially during periods of high activity like controller restarts when many resync events are generated simultaneously.

The priority queue feature addresses the common problem of event storms during resyncs, where hundreds or thousands of objects might be queued at once. By assigning lower priorities to resync events and higher priorities to user-initiated changes, the system remains responsive to important operations.

### Priority levels

The VirtualMachine controller implements five priority levels, evaluated in the following order:

| Priority Level | Value | VM State | Description |
|----------------|-------|----------|-------------|
| **Creating** | `100` | `VirtualMachineConditionCreated` is not `True` | Highest priority. Assigned to VMs that are being created for the first time. This ensures new VM requests are processed quickly. |
| **PowerStateChange** | `99` | `spec.powerState` ≠ `status.powerState` | Second highest priority. Assigned when a VM needs a power state transition (power on, power off, or suspend). Power operations are time-sensitive and user-visible. |
| **Deleting** | `98` | `deletionTimestamp` is set | Assigned to VMs being deleted. Cleanup operations have high priority to free resources promptly. |
| **WaitingForIP** | `97` | Powered on, networking enabled, but no IP address assigned | Assigned when a VM is powered on with networking enabled but hasn't yet received an IP address (neither IPv4 nor IPv6). This ensures the controller actively monitors for network readiness. |
| **WaitingForDiskPromo** | `96` | `spec.promoteDisksMode` ≠ `Disabled` and `VirtualMachineDiskPromotionSynced` is not `True` | Assigned when disk promotion is configured but not yet completed. Ensures timely processing of disk promotion operations. |
| **Default** | `-1` to `-4 `| All other cases | Used for routine reconciliations of stable VMs. The default value varies by event type (see below), with all defaults being negative numbers to indicate lower priority than the specific VM state priorities. |

The **Default** priority level varies based on the Kubernetes event type that triggered the reconciliation:

| Event Type | Default Priority | When Applied |
|------------|------------------|--------------|
| Create | `-1` | Events from initial controller cache sync |
| Update | `-2` | Events caused by an update to the object's desired state |
| Delete | `-3` | Delete events (though typically overridden by state-based priorities) |
| Generic | `-4` | Generic events like periodic resyncs |

This tiered default system ensures that even among routine operations, there's an inherent ordering: create events from cache sync are processed before unchanged updates, which are processed before generic resyncs.

### Priority evaluation

The controller evaluates priorities using a cascade of conditions, with each check returning immediately if matched:

1. **Annotation Override**: If the VM has the `vmoperator.vmware.com.protected/reconcile-priority` annotation set to a valid integer, that value is used directly, bypassing all other checks.

!!! note "Privileged users only"

    Only privileged users, such as the VM Operator service account, may override the reconcile priority via annotation.

2. **Non-VM Objects**: If the object is not a `VirtualMachine`, the default priority is returned.

3. **Deletion**: If the VM has a `deletionTimestamp`, return `PriorityVirtualMachineDeleting` (`98`).

4. **Creation**: If the `VirtualMachineConditionCreated` condition is not `True`, return `PriorityVirtualMachineCreating` (`100`).

5. **Power State Change**: If `spec.powerState` doesn't match `status.powerState`, return `PriorityVirtualMachinePowerStateChange` (`99`).

6. **Waiting for IP**: If the VM is powered on (`status.powerState == PoweredOn`) and networking is enabled (`spec.network.disabled != true`), but no IP address is assigned (`status.network.primaryIP4` and `status.network.primaryIP6` are both empty), return `PriorityVirtualMachineWaitingForIP` (`97`).

7. **Disk Promotion**: If `spec.promoteDisksMode` is not `Disabled` and the `VirtualMachineDiskPromotionSynced` condition is not `True`, return `PriorityVirtualMachineWaitingForDiskPromo` (`96`).

8. **Default**: If none of the above conditions match, return the default priority value that was passed in based on the event type. This means:
    - For **Create** events from initial cache sync: priority `-1`
    - For **Update** events: priority `-2`
    - For **Delete** events (rare, as deletion typically matches condition #3): priority `-3`
    - For **Generic** events (periodic resyncs): priority `-4`

### Priority assignment

Different Kubernetes event types receive different priority handling:

- **Create Events**: During initial controller startup (when processing the initial list of objects from the API server), create events are assigned the default priority. For new objects created after startup, the priority function is evaluated.

- **Update Events**: If the resource version hasn't changed between old and new objects, the default priority is used. Otherwise, the priority function evaluates the VM's current state.

- **Delete Events**: Always evaluated through the priority function, typically resulting in `PriorityVirtualMachineDeleting`.

- **Generic Events**: Always evaluated through the priority function based on the VM's current state.

### Priority examples

**Scenario 1: New VM Creation**
When a user creates a new VirtualMachine resource, the reconcile request receives `PriorityVirtualMachineCreating (100)`, ensuring it's processed ahead of routine updates to existing VMs.

**Scenario 2: Power State Change on Existing VM**
When a user changes `spec.powerState` from `PoweredOff` to `PoweredOn`, the reconcile request receives `PriorityVirtualMachinePowerStateChange (99)`, placing it near the front of the queue.

**Scenario 3: Waiting for IP Address**
After a VM is powered on and networking is configured, but the IP hasn't been assigned yet, subsequent reconciliations receive `PriorityVirtualMachineWaitingForIP (97)`, ensuring the controller actively monitors for IP assignment completion.

**Scenario 4: Routine Reconciliation**
For a stable, running VM with no pending changes, reconcile requests (like periodic resyncs) receive the default priority, allowing more urgent operations to be processed first.

**Scenario 5: Controller Restart with Many VMs**
When the controller starts and processes its initial cache sync with 1,000 VMs, all initial reconciliations receive the default (lower) priority. If during this time a user creates a new VM or requests a power state change, those operations receive higher priority values and are processed first, maintaining system responsiveness.

## Workflows

### Overview

The main `Reconcile` function orchestrates the entire VirtualMachine reconciliation process, handling both creation/updates and deletions:

```mermaid
flowchart TD
    Start([Start Reconcile]) --> GetVM[Get VirtualMachine object]
    GetVM --> GetVMErr{Error getting<br />VM?}
    GetVMErr -->|Yes| ReturnNotFound[Return if not found error]
    GetVMErr -->|No| CheckDeletion{Has deletion<br />timestamp?}

    CheckDeletion -->|Yes| ReconcileDelete[Call ReconcileDelete]
    CheckDeletion -->|No| ReconcileNormal[Call ReconcileNormal]

    ReconcileDelete --> DeleteErr{Delete<br />error?}
    ReconcileNormal --> NormalErr{Normal<br />error?}

    DeleteErr -->|Yes| StoreDeleteErr[Store error in result]
    DeleteErr -->|No| PatchVM[Patch VM object]
    NormalErr -->|Yes| StoreNormalErr[Store error in result]
    NormalErr -->|No| PatchVM

    StoreDeleteErr --> PatchVM
    StoreNormalErr --> PatchVM

    PatchVM --> PatchErr{Patch<br />error?}
    PatchErr -->|Yes| HandlePatchErr[Handle patch error]
    PatchErr -->|No| ReturnResult[Return reconcile result]

    HandlePatchErr --> ReturnResult
    ReturnNotFound --> End([End])
    ReturnResult --> End
```

### Delete

The `ReconcileDelete` method handles the deletion of VirtualMachine resources, ensuring proper cleanup of the underlying vSphere VM and associated resources:

```mermaid
flowchart TD
    Start([Start ReconcileDelete]) --> CheckPause{Has pause<br />annotation?}
    CheckPause -->|Yes| SkipDelete[Skip deletion - VM paused]
    CheckPause -->|No| CheckFinalizer{Has VM<br />finalizer?}

    CheckFinalizer -->|No| CleanupOnly[Only cleanup metrics and probes]
    CheckFinalizer -->|Yes| CheckSkipDelete{Has skip delete<br />annotation?}

    CheckSkipDelete -->|Yes| SkipVMDelete[Skip vSphere VM deletion]
    CheckSkipDelete -->|No| DeleteVSphereVM[Delete vSphere VM]

    DeleteVSphereVM --> DeleteVMErr{VM deletion<br />error?}
    DeleteVMErr -->|Yes| CheckPauseKey{Has pause<br />extraConfig?}
    DeleteVMErr -->|No| RemoveFinalizer[Remove finalizer]

    CheckPauseKey -->|Yes| SkipDelete
    CheckPauseKey -->|No| ReturnDeleteErr[Return deletion error]

    SkipVMDelete --> RemoveFinalizer
    RemoveFinalizer --> CleanupOnly

    CleanupOnly --> DeleteMetrics[Delete VM metrics]
    DeleteMetrics --> RemoveProbe[Remove from probe manager]
    RemoveProbe --> EmitEvent[Emit deletion event]
    EmitEvent --> Success[Deletion completed]

    SkipDelete --> End([End])
    ReturnDeleteErr --> End
    Success --> End
```

### Normal

The `ReconcileNormal` method handles the creation and updating of VirtualMachine resources. It manages finalizers, checks for paused annotations, and orchestrates the main reconcile logic including fast deploy support and VMI cache readiness.

```mermaid
flowchart TD
    Start([Start ReconcileNormal]) --> CheckPause{Has pause<br />annotation?}
    CheckPause -->|Yes| SkipReconcile[Skip reconciliation]
    CheckPause -->|No| CheckFinalizer{Has finalizer?}

    CheckFinalizer -->|No| AddFinalizer[Add finalizer]
    AddFinalizer --> Return([Return - will requeue])

    CheckFinalizer -->|Yes| CheckFastDeploy{Fast Deploy<br />enabled?}
    CheckFastDeploy -->|Yes| CheckVMICache{VMI cache<br />ready?}
    CheckVMICache -->|No| WaitForCache[Wait for VMI cache]
    CheckVMICache -->|Yes| CallProvider[Call VM Provider]
    CheckFastDeploy -->|No| CallProvider

    CallProvider --> CheckAsync{Async enabled?}
    CheckAsync -->|Yes| AsyncCall[CreateOrUpdateVirtualMachineAsync]
    CheckAsync -->|No| BlockingCall[CreateOrUpdateVirtualMachine]

    AsyncCall --> HandleAsync[Handle async response]
    BlockingCall --> HandleBlocking[Handle blocking response]

    HandleAsync --> AddProbe[Add to probe manager]
    HandleBlocking --> AddProbe

    AddProbe --> Return
    WaitForCache --> Return
    SkipReconcile --> Return
```

## vSphere provider

### CreateOrUpdate

The VM provider determines whether to create or update a VM based on whether it exists in vSphere:

```mermaid
flowchart TD
    Start([CreateOrUpdateVirtualMachine]) --> CheckReconciling{Already<br />reconciling?}
    CheckReconciling -->|Yes| ReturnInProgress[Return ErrReconcileInProgress]
    CheckReconciling -->|No| SetVCUUID[Set VC Instance UUID annotation]

    SetVCUUID --> FindVM[Find VM in vSphere]
    FindVM --> VMExists{VM exists?}

    VMExists -->|Yes| MarkUpdate[Mark as Update operation]
    VMExists -->|No| MarkCreate[Mark as Create operation]

    MarkUpdate --> UpdatePath[Update Virtual Machine]
    MarkCreate --> CheckConcurrency{Concurrent creates<br />allowed?}

    CheckConcurrency -->|No| ReturnTooMany[Return ErrTooManyCreates]
    CheckConcurrency -->|Yes| CheckAsyncMode{Is async<br />mode?}

    CheckAsyncMode -->|Yes| StartAsync[Start async create goroutine]
    CheckAsyncMode -->|No| CreateBlocking[Create VM blocking]

    StartAsync --> ReturnChanErr[Return error channel]
    CreateBlocking --> ReturnErr[Return error]
    UpdatePath --> ReturnErr

    ReturnInProgress --> End([End])
    ReturnTooMany --> End
    ReturnChanErr --> End
    ReturnErr --> End
```

### Fast deploy

When creating a VM, the system supports both traditional content library deployment and fast deploy optimization:

```mermaid
flowchart TD
    Start([Create VM]) --> GetArgs[Get create arguments]
    GetArgs --> CheckFastDeploy{Fast Deploy<br />enabled?}

    CheckFastDeploy -->|Yes| CheckVMICache[Check VMI cache ready]
    CheckFastDeploy -->|No| NormalPath[Normal content library path]

    CheckVMICache --> VMICacheReady{VMI cache<br />ready?}
    VMICacheReady -->|No| ReturnNotReady[Return VMICacheNotReadyError]
    VMICacheReady -->|Yes| GetSourcePaths[Get cached source file paths]

    GetSourcePaths --> CheckMode{Fast deploy<br />mode?}
    CheckMode -->|direct| DirectDeploy[Direct deploy from cached disks]
    CheckMode -->|linked| LinkedDeploy[Linked clone from cached disks]

    DirectDeploy --> CopyFiles[Copy cached files to target]
    LinkedDeploy --> CreateLinkedClone[Create linked clone from cache]

    CopyFiles --> CreateVMDirect[Create VM with copied disks]
    CreateLinkedClone --> SetParentRefs[Set parent disk references]
    CreateVMDirect --> SetStatus[Set VM status and conditions]
    SetParentRefs --> CreateVMLinked[Create VM with linked disks]
    CreateVMLinked --> SetStatus

    NormalPath --> DeployFromCL[Deploy from content library]
    DeployFromCL --> SetStatus

    SetStatus --> Success[Return created VM]
    ReturnNotReady --> End([End])
    Success --> End
```

#### VMI Cache Integration (Fast Deploy)

When Fast Deploy is enabled, the controller integrates with VirtualMachineImageCache resources to optimize VM creation:

```mermaid
flowchart TD
    Start([Check VMI Cache Ready]) --> HasLabel{VM has VMI<br />cache label?}
    HasLabel -->|No| Ready[Return ready - no cache needed]
    HasLabel -->|Yes| GetCache[Get VMI cache object]

    GetCache --> CacheExists{Cache<br />exists?}
    CacheExists -->|No| NotReady[Return not ready]
    CacheExists -->|Yes| CheckOVF{OVF condition<br />true?}

    CheckOVF -->|No| NotReady
    CheckOVF -->|Yes| HasLocation{VM has location<br />annotation?}

    HasLocation -->|No| RemoveLabel[Remove VMI cache label]
    HasLocation -->|Yes| FindLocation[Find matching location status]

    RemoveLabel --> Ready
    FindLocation --> LocationReady{Location files<br />ready?}

    LocationReady -->|No| NotReady
    LocationReady -->|Yes| RemoveAnnotations[Remove cache label & annotation]
    RemoveAnnotations --> Ready

    Ready --> End([End - Ready])
    NotReady --> End2([End - Not Ready])
```

This comprehensive workflow documentation shows how the VirtualMachine controller orchestrates VM lifecycle management, including the sophisticated fast deploy optimization that uses cached VM images for faster provisioning.


### Update

When updating an existing VM, the system reconciles various aspects in a specific order:

```mermaid
flowchart TD
    Start([Update VM]) --> FetchProps[Fetch VM properties]
    FetchProps --> UpdateOrder[Update in sequence]

    UpdateOrder --> SchemaUpgrade[Reconcile schema upgrade]
    SchemaUpgrade --> SchemaErr{Schema upgrade<br />error?}
    SchemaErr -->|NoRequeue| ReturnErr[Return error]
    SchemaErr -->|Requeue| AggregateErr[Add to aggregate error]
    SchemaErr -->|Success| Status[Reconcile status]

    Status --> StatusErr{Status update<br />error?}
    StatusErr -->|NoRequeue| ReturnErr
    StatusErr -->|Requeue| AggregateErr
    StatusErr -->|Success| Backup[Reconcile backup state]

    Backup --> BackupErr{Backup error?}
    BackupErr -->|NoRequeue| ReturnErr
    BackupErr -->|Requeue| AggregateErr
    BackupErr -->|Success| Config[Reconcile config]

    Config --> ConfigErr{Config error?}
    ConfigErr -->|NoRequeue & not power off| ReturnErr
    ConfigErr -->|NoRequeue & power off| PowerState[Continue to power state]
    ConfigErr -->|Requeue| AggregateErr
    ConfigErr -->|Success| PowerState

    PowerState --> PowerErr{Power state<br />error?}
    PowerErr -->|NoRequeue| ReturnErr
    PowerErr -->|Requeue| AggregateErr
    PowerErr -->|Success| Snapshot[Reconcile snapshot]

    Snapshot --> SnapshotErr{Snapshot<br />error?}
    SnapshotErr -->|Yes| ReturnSnapshotErr[Return snapshot error]
    SnapshotErr -->|No| CheckAggregateErr{Has aggregate<br />error?}

    AggregateErr --> PowerState
    CheckAggregateErr -->|Yes| ReturnAggregateErr[Return aggregate error]
    CheckAggregateErr -->|No| Success[Update successful]

    ReturnErr --> End([End])
    ReturnSnapshotErr --> End
    ReturnAggregateErr --> End
    Success --> End
```

### Status

The status reconciliation updates the VM's observed state from vSphere:

```mermaid
flowchart TD
    Start([Reconcile Status]) --> SetCreatedCond[Set Created condition to true]
    SetCreatedCond --> CheckClassless{Is classless<br />VM?}

    CheckClassless -->|Yes| ClearClass[Clear status.class]
    CheckClassless -->|No| CheckResize{Resize<br />enabled?}

    CheckResize -->|Yes| SkipClassBackfill[Skip class backfill]
    CheckResize -->|No| BackfillClass[Backfill class from spec]

    ClearClass --> UpdateFields[Update core status fields]
    SkipClassBackfill --> UpdateFields
    BackfillClass --> UpdateFields

    UpdateFields --> UpdateGroupCond[Update group-linked condition]
    UpdateFields --> SetPowerState[Set power state]
    UpdateFields --> SetIDs[Set UniqueID, BiosUUID, InstanceUUID]
    UpdateFields --> SetHWVersion[Set hardware version]
    UpdateFields --> UpdateNetworkStatus[Update network status]
    UpdateFields --> UpdateStorageStatus[Update storage status]

    UpdateGroupCond --> CheckAsync{Async signal<br />enabled?}
    SetPowerState --> CheckAsync
    SetIDs --> CheckAsync
    SetHWVersion --> CheckAsync
    UpdateNetworkStatus --> CheckAsync
    UpdateStorageStatus --> CheckAsync

    CheckAsync -->|Yes| UpdateProbeStatus[Update probe status]
    CheckAsync -->|No| GetHostname[Get runtime host hostname]

    UpdateProbeStatus --> GetHostname
    GetHostname --> SetNodeName[Set status.nodeName]
    SetNodeName --> UpdateEvents[Update VM events]
    UpdateEvents --> Success[Status reconciled]

    Success --> End([End])
```

### Config

Config reconciliation ensures the VM configuration matches the desired spec:

```mermaid
flowchart TD
    Start([Reconcile Config]) --> Verify[Verify prerequisites]
    Verify --> VerifyConfigInfo[Verify config info]
    VerifyConfigInfo --> VerifyConnection[Verify connection state]
    VerifyConnection --> VerifyRP[Verify resource pool]

    VerifyRP --> GetCluster[Get cluster MoRef from resource pool]
    GetCluster --> CreateSession[Create vSphere session]
    CreateSession --> GetUpdateArgs[Get VM update arguments]

    GetUpdateArgs --> GetResizeArgs[Get VM resize arguments]
    GetResizeArgs --> CallSession[Call session.UpdateVirtualMachine]

    CallSession --> CheckResize{Needs<br />resize?}
    CheckResize -->|Yes| ResizeVM[Resize VM CPU/memory]
    CheckResize -->|No| UpdateConfig[Update VM configuration]

    ResizeVM --> UpdateConfig
    UpdateConfig --> Success[Config reconciled]

    Success --> End([End])
```

### Power state

Power state reconciliation manages VM power operations:

```mermaid
flowchart TD
    Start([Reconcile Power State]) --> VerifyConnection[Verify connection state]
    VerifyConnection --> VerifyRP[Verify resource pool]
    VerifyRP --> CheckGroups{VM Groups<br />enabled?}

    CheckGroups -->|Yes| CheckPowerOn{Desired state<br />is PowerOn?}
    CheckGroups -->|No| ComparePowerState[Compare current vs desired]

    CheckPowerOn -->|Yes| CheckDelayed{Has delayed<br />power state?}
    CheckPowerOn -->|No| ComparePowerState

    CheckDelayed -->|Yes| CheckTime{Time<br />arrived?}
    CheckDelayed -->|No| ComparePowerState

    CheckTime -->|No| WaitForTime[Return with requeue delay]
    CheckTime -->|Yes| ComparePowerState

    ComparePowerState --> NeedsPowerOp{Needs power<br />operation?}
    NeedsPowerOp -->|No| Success[Power state reconciled]
    NeedsPowerOp -->|Yes| ExecutePowerOp[Execute power operation]

    ExecutePowerOp --> PowerOnOff{Power On<br />or Off?}
    PowerOnOff -->|On| PowerOn[Power on VM]
    PowerOnOff -->|Off| PowerOff[Power off VM]
    PowerOnOff -->|Suspend| Suspend[Suspend VM]

    PowerOn --> WaitForTools{Wait for<br />VMware Tools?}
    PowerOff --> Success
    Suspend --> Success

    WaitForTools -->|Yes| CheckTools[Check VMware Tools status]
    WaitForTools -->|No| Success

    CheckTools --> ToolsReady{Tools<br />ready?}
    ToolsReady -->|Yes| Success
    ToolsReady -->|No| WaitMore[Continue waiting]

    WaitForTime --> End([End])
    WaitMore --> End
    Success --> End
```

