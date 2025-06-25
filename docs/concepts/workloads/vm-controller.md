# VirtualMachine Controller

The `VirtualMachine` controller is responsible for reconciling `VirtualMachine` objects.

## Reconcile

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

## ReconcileDelete

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

### ReconcileNormal

The `ReconcileNormal` method handles the creation and updating of VirtualMachine resources. It manages finalizers, checks for paused annotations, and orchestrates the main reconcile logic including fast deploy support and VMI cache readiness.

#### High-Level Reconcile Normal Flow

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

#### VM Provider CreateOrUpdate Decision Logic

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

#### VM Creation Workflow with Fast Deploy

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

#### VM Update Workflow

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

#### Status Reconciliation Details

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

#### Config Reconciliation Process

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

#### Power State Reconciliation

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

### VMI Cache Integration (Fast Deploy)

When Fast Deploy is enabled, the controller integrates with VirtualMachineImageCache resources to optimize VM creation:

#### VMI Cache Readiness Check

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
