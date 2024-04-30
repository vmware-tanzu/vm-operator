# VirtualMachine Controller

The `VirtualMachine` controller is responsible for reconciling `VirtualMachine` objects.

## Reconcile

```mermaid
flowchart LR
    Start(Start) --> GetVM

    GetVM(Get VM) --> GetVMErr{Error?}
    GetVMErr -->|No| HasDeletionTimestamp{Has deletion\ntimestamp?}
    GetVMErr ---->|Yes| StoreGetErrAndReturnErr

    HasDeletionTimestamp --> |Yes| ReconcileDelete
    HasDeletionTimestamp ----> |No| ReconcileNormal

    ReconcileDelete(Reconcile delete) --> ReconcileDeleteNormalErr
    ReconcileNormal(Reconcile normal) --> ReconcileDeleteNormalErr
    ReconcileDeleteNormalErr{Error?}
    ReconcileDeleteNormalErr --> |No| PatchVM
    ReconcileDeleteNormalErr ----> |Yes| StoreErrAndPatch
    StoreErrAndPatch(Store reconcile error\nin <code>result</code>) --> PatchVM

    PatchVM(Patch VM) --> PatchVMErr{Error?}
    PatchVMErr --> |Yes| IsResultErr{Is error already\nstored in <code>result</code>?}
    PatchVMErr ----> |No| ReturnResult
    IsResultErr --> |No| StorePatchErr
    IsResultErr ----> |Yes| LogPatchErr
    LogPatchErr(Log patch error) --> ReturnResult
    StorePatchErr(Store patch error\nin <code>result</code>) --> ReturnResult

    %% This element's position keeps the graph from having to transect points
    %% between nodes.
    StoreGetErrAndReturnErr(Store get error\nin <code>result</code>) --> ReturnResult

    ReturnResult(Return <code>result</code>) --> End

    End(End)
```

### Reconcile Delete

```mermaid
flowchart LR
    Start(Start) --> HasPauseAnnotation{Has pause\nannotation?}
    HasPauseAnnotation --> |No| HasFinalizer{Has finalizer?}
    HasPauseAnnotation ----> |Yes| End
    

    HasFinalizer --> |No| DeleteMetrics(Delete metrics)
    HasFinalizer ----> |Yes| DeleteVM(Delete vSphere VM)

    DeleteVM --> DeleteVMErr{Error?}
    DeleteVMErr --> |No| RemoveFinalizer(Remove finalizer)
    DeleteVMErr ----> |Yes| ErrorVMPausedByAdmin{Has pause\nextraConfig key?}
    
    ErrorVMPausedByAdmin --> |Yes| End
    ErrorVMPausedByAdmin --> |No| ReturnErr(Return error)

    RemoveFinalizer --> DeleteMetrics(Delete metrics)

    DeleteMetrics --> RemoveProbe(Remove probe)
    RemoveProbe --> End

    ReturnErr --> End

    End(End)
```

### Reconcile Normal

// TODO ([github.com/vmware-tanzu/vm-operator#444](https://github.com/vmware-tanzu/vm-operator/issues/444))
