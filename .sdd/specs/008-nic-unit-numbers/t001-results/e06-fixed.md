# NIC unit numbers — govmomi research results (T001)

## Environment

| Property | Value |
|---|---|
| Run at | 2026-08-23T15:09:04Z |
| vCenter | 10.162.38.193 (VMware vCenter Server 9.2.0.0.25689988) |
| vCenter version | 9.2.0 |
| vCenter build | 25689988 |
| vCenter API version | 9.1.2.0.rc0 |
| ESX host | 10.162.34.186 |
| ESX version | 9.2.0 |
| ESX build | 25690016 |
| VM hardware version | vmx-23 |
| Datacenter | /dc |
| Resource pool | /dc/host/dc-cluster/Resources |
| Datastore | /dc/datastore/sharedVmfs-0 |
| Folder | /dc/vm |
| Network | VM Network |
| Support matrix covered | vCenter 9.2.0 build 25689988 / ESX 9.2.0 build 25690016 (E06 rerun with corrected finding text) |
| govmomi | v0.56.0-alpha.0.0.20260720221020-d993be43fe66 |

> A single-vCenter run does not answer cross-version stability. Treat every result below as characterising the builds named above only (R6).

## Summary

| Experiment | Question(s) | Status | Title |
|---|---|---|---|
| E06 |  | HONOURED | Remove at unit N and Add at unit N are accepted in one ReconfigVM_Task |

## Results

### E06 — Remove at unit N and Add at unit N are accepted in one ReconfigVM_Task

**Status**: HONOURED

#### Step: Initial hardware (NICs at units 7 and 9)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:e5:f7/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:aa:29/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Remove the NIC at unit 9 and add a new one at unit 9 in one task

Requested:

```
unit=9 key=4002 controllerKey=100 kind=remove VirtualVmxnet3 mac=00:50:56:a7:aa:29/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=-1788463697 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:e5:f7/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:47:b0/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Removed device: unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:aa:29/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.
- The device at unit 9 after the task has key 4002 and MAC "00:50:56:a7:47:b0"; the removed device had key 4002 and MAC "00:50:56:a7:aa:29" (key changed: false, MAC changed: true). A changed MAC — and, on builds where the key is not derived from the unit number, a changed key — confirms the slot was genuinely reused by new hardware rather than the remove being ignored.
- Key was UNCHANGED across the same-slot Remove+Add (both 4002). This build appears to derive an ethernet card's Key deterministically from its unit number rather than from creation order; do not rely on a changed Key alone as replacement evidence on this platform.
