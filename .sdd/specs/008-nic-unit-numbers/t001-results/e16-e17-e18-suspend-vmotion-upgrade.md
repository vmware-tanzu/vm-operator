# NIC unit numbers — govmomi research results (T001)

## Environment

| Property | Value |
|---|---|
| Run at | 2026-08-24T18:44:24Z |
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
| Support matrix covered | _(not recorded)_ |
| govmomi | v0.56.0-alpha.0.0.20260720221020-d993be43fe66 |

> A single-vCenter run does not answer cross-version stability. Treat every result below as characterising the builds named above only (R6).

## Summary

| Experiment | Question(s) | Status | Title |
|---|---|---|---|
| E16 |  | RECORDED | NIC unit numbers are stable across a suspend/resume cycle |
| E17 |  | RECORDED | NIC unit numbers are stable across vMotion to another host |
| E18 |  | RECORDED | NIC unit numbers are stable across hardware-version (VM Compatibility) upgrades, including after powering on at each new version |

## Results

### E16 — NIC unit numbers are stable across a suspend/resume cycle

**Status**: RECORDED

#### Step: Units powered on, before suspend

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:82:6a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:74:c6/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:ec:fa/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units while suspended

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:82:6a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:74:c6/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:ec:fa/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units after resume

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:82:6a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:74:c6/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:ec:fa/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- Unit numbers were unchanged across suspend/resume: [7 9 12].

### E17 — NIC unit numbers are stable across vMotion to another host

**Status**: RECORDED

#### Step: Units before vMotion

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:91:7a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:bf:67/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:2b:6a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: vMotion from 10.162.36.16 to 10.162.37.144

#### Step: Units after vMotion

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:91:7a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:bf:67/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:2b:6a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- VM ran on host "10.162.36.16" before, "10.162.37.144" after.
- Unit numbers were unchanged across vMotion: [7 9 12].

### E18 — NIC unit numbers are stable across hardware-version (VM Compatibility) upgrades, including after powering on at each new version

**Status**: RECORDED

#### Step: Units at hardware version vmx-15, before any upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: UpgradeVM_Task from vmx-15 toward vmx-17

#### Step: Units at hardware version vmx-17, powered off, right after upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units at hardware version vmx-17, powered ON after upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: UpgradeVM_Task from vmx-17 toward vmx-20

#### Step: Units at hardware version vmx-20, powered off, right after upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units at hardware version vmx-20, powered ON after upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: UpgradeVM_Task from vmx-20 toward the host's maximum supported version

#### Step: Units at hardware version vmx-23, powered off, right after upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units at hardware version vmx-23, powered ON after upgrade

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:67:72/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:88:bb/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:1e:10/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- Hardware version was "vmx-15" before this upgrade step, "vmx-17" after.
- Unit numbers were unchanged by the upgrade from vmx-15 to "vmx-17" (still powered off): [7 9 12].
- Unit numbers were unchanged by powering on at hardware version "vmx-17": [7 9 12].
- Hardware version was "vmx-17" before this upgrade step, "vmx-20" after.
- Unit numbers were unchanged by the upgrade from vmx-17 to "vmx-20" (still powered off): [7 9 12].
- Unit numbers were unchanged by powering on at hardware version "vmx-20": [7 9 12].
- Hardware version was "vmx-20" before this upgrade step, "vmx-23" after.
- Unit numbers were unchanged by the upgrade from vmx-20 to "vmx-23" (still powered off): [7 9 12].
- Unit numbers were unchanged by powering on at hardware version "vmx-23": [7 9 12].
- Unit numbers were unchanged end-to-end, from vmx-15 through every upgrade and power-cycle step above: [7 9 12].
