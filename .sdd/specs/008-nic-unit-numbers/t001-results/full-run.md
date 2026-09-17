# NIC unit numbers — govmomi research results (T001)

## Environment

| Property | Value |
|---|---|
| Run at | 2026-08-23T14:59:40Z |
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
| Support matrix covered | vCenter 9.2.0 build 25689988 / ESX 9.2.0 build 25690016 (wcp-4-esx-fullInstall testbed, nested/nimbus ESXi - no SR-IOV or vGPU/DVX passthrough hardware available) |
| govmomi | v0.56.0-alpha.0.0.20260720221020-d993be43fe66 |

> A single-vCenter run does not answer cross-version stability. Treat every result below as characterising the builds named above only (R6).

## Summary

| Experiment | Question(s) | Status | Title |
|---|---|---|---|
| E01 | Q1 | HONOURED | folder.CreateVM honours explicit NIC unit numbers in the create ConfigSpec |
| E02 | Q1, Q2 | HONOURED | OVF content-library deploy honours explicit NIC unit numbers |
| E03 | Q1 | HONOURED | ReconfigVM_Task Add honours explicit NIC unit numbers on a powered-off VM |
| E04 |  | RECORDED | NICs added with no unit number are assigned from 7 upward |
| E05 |  | RECORDED | Add and remove NICs, powered off (primary) and powered on (informational) |
| E06 |  | HONOURED | Remove at unit N and Add at unit N are accepted in one ReconfigVM_Task |
| E07 |  | RECORDED | Edit an existing NIC's unit number on a powered-off VM (informational) |
| E08 | Q4 | RECORDED | Fault returned when an explicit NIC unit number collides |
| E09 |  | RECORDED | ControllerKey on operator-built Add payloads is unset and resolved by vSphere |
| E10 |  | SKIPPED | NICs keep the 7-16 band on a PCI bus shared with a passthrough device |
| E11 | Q5 | RECORDED | Does an auto-assigned NIC reuse a freed unit number? |
| E12 | Q6 | RECORDED | NIC unit numbers are stable across a power cycle |
| E13 | Q7 | RECORDED | Out-of-band NIC add through the vCenter UI |
| E14 | Q3 | SKIPPED | SR-IOV ethernet cards share the 7-16 unit-number space |
| E15 |  | HONOURED | Hot-add a NIC with an explicit unit number (informational) |

## Results

### E01 — folder.CreateVM honours explicit NIC unit numbers in the create ConfigSpec

**Answers**: Q1

**Status**: HONOURED

#### Step: CreateVM with NICs at units [7 10 16]

Requested:

```
unit=7 key=-944671563 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=10 key=-153904398 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=-1368108760 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:44:d0/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=10 key=4003 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:0f:cf/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=4009 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:40:8b/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.

### E02 — OVF content-library deploy honours explicit NIC unit numbers

**Answers**: Q1, Q2

**Status**: HONOURED

#### Step: Deploy the OVF with no ConfigSpec NIC entries (baseline)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:c1:8b/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

These are the NICs the OVF descriptor itself contributes, and the slots the platform gave them with no ConfigSpec involvement.

#### Step: Deploy the OVF with ConfigSpec NIC Adds at units [7 10 16]

Requested:

```
unit=7 key=-363963930 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=10 key=-147322480 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=-720614886 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:0b:46/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=10 key=4003 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:9f:2a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=4009 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:6e:7a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Compare against the baseline step: 1 NIC(s) came from the OVF alone, 3 are present here.

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.
- The NIC count is not baseline + ConfigSpec adds (3 != 1 + 3): the OVF's own NICs and the ConfigSpec Add entries interact rather than accumulate. Record which.

### E03 — ReconfigVM_Task Add honours explicit NIC unit numbers on a powered-off VM

**Answers**: Q1

**Status**: HONOURED

#### Step: Add NICs at units [7 10 16] to a powered-off VM

Requested:

```
unit=7 key=-2041516155 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=10 key=-213077978 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=-637889678 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:f0:19/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=10 key=4003 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:de:b3/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=16 key=4009 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:10:7e/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.

### E04 — NICs added with no unit number are assigned from 7 upward

**Status**: RECORDED

#### Step: Add the first NIC with UnitNumber nil

Requested:

```
unit=nil (auto) key=-374998756 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:6c:5a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Add a second NIC with UnitNumber nil

Requested:

```
unit=nil (auto) key=-1784414495 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:6c:5a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:e9:49/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- First NIC landed at unit 7 (expected 7).
- Second NIC landed at unit 8.
- Every observed unit number must fall in 7-16 for the CRD range markers in T004 to be correct.

### E05 — Add and remove NICs, powered off (primary) and powered on (informational)

**Status**: RECORDED

#### Step: Initial hardware (two auto-assigned NICs)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:c8:24/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:1a:75/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Remove the second NIC while powered off

Requested:

```
unit=8 key=4001 controllerKey=100 kind=remove VirtualVmxnet3 mac=00:50:56:a7:1a:75/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:c8:24/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Add a NIC with UnitNumber nil while powered off

Requested:

```
unit=nil (auto) key=-802999844 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:c8:24/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:13:92/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Power on for the informational hot-add/hot-remove steps

#### Step: Hot-add a NIC with UnitNumber nil (informational)

Requested:

```
unit=nil (auto) key=-560918710 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:c8:24/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:13:92/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:b6:77/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Informational only — the product emits no NIC device changes on a powered-on VM (I5).

#### Step: Hot-remove the last NIC (informational)

Requested:

```
unit=9 key=4002 controllerKey=100 kind=remove VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:b6:77/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:c8:24/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:13:92/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:b6:77/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `ReconfigVM_Task failed: The guest operating system did not respond to a hot-remove request for device ethernet2 in a timely manner.`

Fault: `GenericVmConfigFault`

> The guest operating system did not respond to a hot-remove request for device ethernet2 in a timely manner.
>
> msg.vigor.hotRemoveStillExists: The guest operating system did not respond to a hot-remove request for device ethernet2 in a timely manner.
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEZaG9zdGQAAIKtV2xpYnZtYWNvcmUuc28AADP3OACo7DqBVbJ5AWhvc3RkAIEv/oEBAo3QDGxpYnZpZ29yLnNvAAIG0gwCoZ0SAvv/IwLDTSQDYf4ObGlidm1zbmFwc2hvdC5zbwAD9qoPA1WtDwNKug+BiGxeAQAvODoAJrwjAI69IwTrFxVsaWJ2YXBpLWNvcmUuc28uMgAAC4kkANJpJQBtbCUA1yZVBXdoCGxpYmMuc28uNgAFgGkQ[/context]
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEcdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90eBDMR1AWxpYnZpbS10eXBlcy5zbwCBDQJ2AQItbR12cHhhAAK/+B4CT/0eAujqNgLgSC2BMR4rAQOnfSdsaWJ2bW9taS5zbwACpKMgA0ObGwKZzS4CkTceAkL+HQKfcB4C/IYeAsHXHQI53x4AC4kkANJpJQBtbCUA1yZVBHdoCGxpYmMuc28uNgAEgGkQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwExdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBOj2VBIECPpUEgQXZGQSB5dwZBIF73RkEgXMZGgSCDb2OAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEpdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdggR9b2AFsaWJ2aW0tdHlwZXMuc28Agb+n2AGCNdxFAnZweGQAgkdVKwWCSz2VBIICPpUEggXZGQSC5dwZBIJ73RkEgnMZGgSBDb2OAQPo8zJsaWJ2bW9taS5zbwCCvAsvBYL0DS8FAwvgKILXOJsCgoFM4ASC8agsBYJbqiwFgv1bKwWCm6osBYL+tCwFgqA7KwWCW1srBYKmWysFgp/YLAWCpj5DAgB/yD4A5I9AAAJaQQBGXEEAe1xBAH/IPgAdimsEm+cIbGliYy5zby42AAR4cBA=[/context]

### E06 — Remove at unit N and Add at unit N are accepted in one ReconfigVM_Task

**Status**: HONOURED

#### Step: Initial hardware (NICs at units 7 and 9)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:15:58/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:8d:11/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Remove the NIC at unit 9 and add a new one at unit 9 in one task

Requested:

```
unit=9 key=4002 controllerKey=100 kind=remove VirtualVmxnet3 mac=00:50:56:a7:8d:11/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=-368901601 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:15:58/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:95:b6/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Removed device: unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:8d:11/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.
- The device at unit 9 after the task has key 4002 and MAC "00:50:56:a7:95:b6"; the removed device had key 4002 and MAC "00:50:56:a7:8d:11". A changed key confirms the slot was genuinely reused by new hardware rather than the remove being ignored.

### E07 — Edit an existing NIC's unit number on a powered-off VM (informational)

**Status**: RECORDED

#### Step: Edit the NIC at unit 7 to unit 11

Requested:

```
unit=11 key=4000 controllerKey=100 kind=edit VirtualVmxnet3 mac=00:50:56:a7:d9:3c/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=11 key=4004 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:d9:3c/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Informational only: nothing in this change set issues an Edit that relocates a unit number.

**Findings**:

- The Edit was accepted and the NIC now occupies unit 11.

### E08 — Fault returned when an explicit NIC unit number collides

**Answers**: Q4

**Status**: RECORDED

#### Step: CreateVM with two NICs both at the same unit number

Requested:

```
unit=7 key=-1632497346 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=7 key=-1541423239 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `CreateVM failed for nic-unit-research-e08-tk89ne-create-collision: Invalid configuration for device '1'.`

Fault: `InvalidDeviceSpec` deviceIndex=`1` property=`unitNumber`

> Invalid configuration for device '1'.
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEbaG9zdGQAAIKtV2xpYnZtYWNvcmUuc28AADP3OACo7DoAAfdHAe2fZWhvc3RkAIGMumEBgdxMfQGBc8NnAYFwzmcBgSzbZwGBz+JnAYEAk2IBgenVbQGBf+RtAYE00nQBgUDXdAEBqG7Ygk+FKwFsaWJ2aW0tdHlwZXMuc28AA6d9J2xpYnZtb21pLnNvAAE7PG4E6xcVbGlidmFwaS1jb3JlLnNvLjIAAAuJJADSaSUAbWwlANcmVQV3aAhsaWJjLnNvLjYABYBpEA==[/context]
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEUdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90eBDMR1AWxpYnZpbS10eXBlcy5zbwCBoQp2AQILHS92cHhhAAKUBB4CvMQdAkL+HQKfcB4C04keAsHXHQI53x4AC4kkANJpJQBtbCUA1yZVA3doCGxpYmMuc28uNgADgGkQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEudnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBC0CVBIF+WZUEgW7kRQKCpUePAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEmdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdggR9b2AFsaWJ2aW0tdHlwZXMuc28AgYey2AGCNdxFAnZweGQAgkdVKwWCHECVBIJ+WZUEgm7kRQKBpUePAQPo8zJsaWJ2bW9taS5zbwCCvAsvBYL0DS8FAwvgKILXOJsCgoFM4ASC8agsBYJbqiwFgv1bKwWCm6osBYL+tCwFgqA7KwWCW1srBYKmWysFgp/YLAWCpj5DAgB/yD4A5I9AAAJaQQBGXEEAe1xBAH/IPgAdimsEm+cIbGliYy5zby42AAR4cBA=[/context]

#### Step: ReconfigVM_Task Add at a unit number an existing NIC already occupies

Requested:

```
unit=7 key=-428344214 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:8c:f9/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `ReconfigVM_Task failed: Invalid configuration for device '0'.`

Fault: `InvalidDeviceSpec` deviceIndex=`0` property=`unitNumber`

> Invalid configuration for device '0'.
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEcdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90eBDMR1AWxpYnZpbS10eXBlcy5zbwCBNQp2AQItbR12cHhhAAK/+B4CT/0eAujqNgLgSC2BMR4rAQOnfSdsaWJ2bW9taS5zbwACpKMgA0ObGwKZzS4CkTceAkL+HQKfcB4C/IYeAsHXHQI53x4AC4kkANJpJQBtbCUA1yZVBHdoCGxpYmMuc28uNgAEgGkQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwExdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBOj2VBIECPpUEgQXZGQSB5dwZBIF73RkEgXMZGgSCDb2OAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEpdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdggR9b2AFsaWJ2aW0tdHlwZXMuc28AgYey2AGCNdxFAnZweGQAgkdVKwWCSz2VBIICPpUEggXZGQSC5dwZBIJ73RkEgnMZGgSBDb2OAQPo8zJsaWJ2bW9taS5zbwCCvAsvBYL0DS8FAwvgKILXOJsCgoFM4ASC8agsBYJbqiwFgv1bKwWCm6osBYL+tCwFgqA7KwWCW1srBYKmWysFgp/YLAWCpj5DAgB/yD4A5I9AAAJaQQBGXEEAe1xBAH/IPgAdimsEm+cIbGliYy5zby42AAR4cBA=[/context]

**Findings**:

- Step "CreateVM with two NICs both at the same unit number" returned fault `InvalidDeviceSpec` with deviceIndex 1.
- Step "ReconfigVM_Task Add at a unit number an existing NIC already occupies" returned fault `InvalidDeviceSpec` with deviceIndex 0.

### E09 — ControllerKey on operator-built Add payloads is unset and resolved by vSphere

**Status**: RECORDED

#### Step: CreateVM with an operator-shaped NIC (ControllerKey left unset)

Requested:

```
unit=nil (auto) key=-2014313569 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:33:af/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

PCI controller: unit=nil (auto) key=100 controllerKey=0 kind=VirtualPCIController

**Findings**:

- The requested payload carried ControllerKey=0 (unset).
- vSphere resolved the NIC onto the PCI controller (key 100) with no operator involvement. No design change follows.

### E10 — NICs keep the 7-16 band on a PCI bus shared with a passthrough device

**Status**: SKIPPED

**Reason**: no -vgpu-profile or -dvx-device-class supplied; this experiment needs a passthrough-capable host and a device to attach

### E11 — Does an auto-assigned NIC reuse a freed unit number?

**Answers**: Q5

**Status**: RECORDED

#### Step: Initial hardware (three auto-assigned NICs)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:6e:82/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:8a:de/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:44:99/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Remove the middle NIC at unit 8

Requested:

```
unit=8 key=4001 controllerKey=100 kind=remove VirtualVmxnet3 mac=00:50:56:a7:8a:de/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:6e:82/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:44:99/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Add a NIC with UnitNumber nil

Requested:

```
unit=nil (auto) key=-1830559345 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:6e:82/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:44:99/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:3e:ce/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- vSphere REUSED the freed unit 8 for the new NIC.

### E12 — NIC unit numbers are stable across a power cycle

**Answers**: Q6

**Status**: RECORDED

#### Step: Units before the power cycle

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:87:fd/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:27:52/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:20:cf/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units while powered on

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:87:fd/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:27:52/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:20:cf/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units after powering back off

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:87:fd/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:27:52/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=224 mac=00:50:56:a7:20:cf/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- Unit numbers were unchanged across the power cycle: [7 9 12].

### E13 — Out-of-band NIC add through the vCenter UI

**Answers**: Q7

**Status**: RECORDED

#### Step: Units before the out-of-band add (NICs at 7 and 9)

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:4d:42/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:2d:03/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: Units after the out-of-band add

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:4d:42/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=9 key=4002 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:2d:03/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:07:50/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

**Findings**:

- The existing NICs kept units [7 9]; the out-of-band NIC took the remainder of [7 9 8].

### E14 — SR-IOV ethernet cards share the 7-16 unit-number space

**Answers**: Q3

**Status**: SKIPPED

**Reason**: no -sriov-network supplied; this experiment needs an SR-IOV-capable pNIC/host and a suitable network (R5)

### E15 — Hot-add a NIC with an explicit unit number (informational)

**Status**: HONOURED

#### Step: Hot-add a NIC at explicit unit 12

Requested:

```
unit=12 key=-605430151 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 pciSlot=160 mac=00:50:56:a7:80:ea/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=12 key=4005 controllerKey=100 kind=VirtualVmxnet3 pciSlot=192 mac=00:50:56:a7:12:c4/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Informational only — this change set never emits a NIC device change on a powered-on VM (I5).

**Findings**:

- Every explicitly requested unit number was observed on the resulting hardware.
