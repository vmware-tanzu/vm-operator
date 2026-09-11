# NIC unit numbers — govmomi research results (T001)

## Environment

| Property | Value |
|---|---|
| Run at | 2026-08-23T15:06:33Z |
| vCenter | 10.162.38.193 (VMware vCenter Server 9.2.0.0.25689988) |
| vCenter version | 9.2.0 |
| vCenter build | 25689988 |
| vCenter API version | 9.1.2.0.rc0 |
| ESX host | 10.162.34.186 |
| ESX version | 9.2.0 |
| ESX build | 25690016 |
| VM hardware version | vmx-15 |
| Datacenter | /dc |
| Resource pool | /dc/host/dc-cluster/Resources |
| Datastore | /dc/datastore/sharedVmfs-0 |
| Folder | /dc/vm |
| Network | VM Network |
| Support matrix covered | vCenter 9.2.0 build 25689988 / ESX 9.2.0 build 25690016 (E02 discriminator rerun, units [10,16], excluding 7) |
| govmomi | v0.56.0-alpha.0.0.20260720221020-d993be43fe66 |

> A single-vCenter run does not answer cross-version stability. Treat every result below as characterising the builds named above only (R6).

## Summary

| Experiment | Question(s) | Status | Title |
|---|---|---|---|
| E02 | Q1, Q2 | HONOURED | OVF content-library deploy honours explicit NIC unit numbers |

## Results

### E02 — OVF content-library deploy honours explicit NIC unit numbers

**Answers**: Q1, Q2

**Status**: HONOURED

#### Step: Deploy the OVF with no ConfigSpec NIC entries (baseline)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:78:dc/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

These are the NICs the OVF descriptor itself contributes, and the slots the platform gave them with no ConfigSpec involvement.

#### Step: Deploy the OVF with ConfigSpec NIC Adds at units [10 16]

Requested:

```
unit=10 key=-639861565 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=-1889164362 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=10 key=4003 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:79:b3/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=4009 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:04:f4/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Compare against the baseline step: 1 NIC(s) came from the OVF alone, 2 are present here.

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.
- The NIC count is not baseline + ConfigSpec adds (2 != 1 + 2): the OVF's own NICs and the ConfigSpec Add entries interact rather than accumulate. Record which.
