# NIC unit numbers — govmomi research results (T001)

## Environment

| Property | Value |
|---|---|
| Run at | 2026-08-31T20:01:12Z |
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
| Network | /dc/network/VM Network |
| Support matrix covered | _(not recorded)_ |
| govmomi | v0.56.0-alpha.0.0.20260720221020-d993be43fe66 |

> A single-vCenter run does not answer cross-version stability. Treat every result below as characterising the builds named above only (R6).

## Summary

| Experiment | Question(s) | Status | Title |
|---|---|---|---|
| E19 |  | RECORDED | vSphere's handling of an explicit NIC UnitNumber outside the 7-16 band |

## Results

### E19 — vSphere's handling of an explicit NIC UnitNumber outside the 7-16 band

**Status**: RECORDED

#### Step: CreateVM with a NIC at out-of-range unit 3

Requested:

```
unit=3 key=-676829513 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:9b:35/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: ReconfigVM_Task Add at out-of-range unit 3

Requested:

```
unit=3 key=-2036053110 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:71:4c/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
unit=8 key=4001 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:f5:50/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

#### Step: CreateVM with a NIC at out-of-range unit 17

Requested:

```
unit=17 key=-2090104660 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `CreateVM failed for nic-unit-research-e19-tkngxy-create-oor-17: A specified parameter was not correct: unitNumber`

Fault: `InvalidArgument`

> A specified parameter was not correct: unitNumber
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEcaG9zdGQAAIKtV2xpYnZtYWNvcmUuc28AADP3OACo7DoAAfdHAe2fZWhvc3RkAAF6p2WBjO98AYFsTH0BgXPDZwGBcM5nAYEs22cBgc/iZwGBAJNiAYHp1W0BgX/kbQGBNNJ0AYFA13QBAahu2IJPhSsBbGlidmltLXR5cGVzLnNvAAOnfSdsaWJ2bW9taS5zbwABOzxuBOsXFWxpYnZhcGktY29yZS5zby4yAAALiSQA0mklAG1sJQDXJlUFd2gIbGliYy5zby42AAWAaRA=[/context]
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwETdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90cBJbctbGlidm1vbWkuc28AAgsdL3ZweGEAApQEHgK8xB0CQv4dAp9wHgLTiR4CwdcdAjnfHgALiSQA0mklAG1sJQDXJlUDd2gIbGliYy5zby42AAOAaRA=[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEudnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBC0CVBIF+WZUEgW7kRQKCpUePAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEmdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdgAcEYIWxpYnZtb21pLnNvAAF/pTiCNdxFAnZweGQAgkdVKwWCHECVBIJ+WZUEgm7kRQKDpUePAWxpYnZpbS10eXBlcy5zbwAB6PMygrwLLwWC9A0vBQEL4CiC1zibAoKBTOAEgvGoLAWCW6osBYL9WysFgpuqLAWC/rQsBYKgOysFgltbKwWCplsrBYKf2CwFgqY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]

#### Step: ReconfigVM_Task Add at out-of-range unit 17

Requested:

```
unit=17 key=-1483942035 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:53:8a/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `ReconfigVM_Task failed: A specified parameter was not correct: unitNumber`

Fault: `InvalidArgument`

> A specified parameter was not correct: unitNumber
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEcdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90cBErUTbGlidm1vbWkuc28AAVHXLQItbR12cHhhAAK/+B4CT/0eAujqNgLgSC2DMR4rAWxpYnZpbS10eXBlcy5zbwABp30nAqSjIAFDmxsCmc0uApE3HgJC/h0Cn3AeAvyGHgLB1x0COd8eAAuJJADSaSUAbWwlANcmVQR3aAhsaWJjLnNvLjYABIBpEA==[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwExdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBOj2VBIECPpUEgQXZGQSB5dwZBIF73RkEgXMZGgSCDb2OAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEpdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdgAcEYIWxpYnZtb21pLnNvAAF/pTiCNdxFAnZweGQAgkdVKwWCSz2VBIICPpUEggXZGQSC5dwZBIJ73RkEgnMZGgSDDb2OAWxpYnZpbS10eXBlcy5zbwAB6PMygrwLLwWC9A0vBQEL4CiC1zibAoKBTOAEgvGoLAWCW6osBYL9WysFgpuqLAWC/rQsBYKgOysFgltbKwWCplsrBYKf2CwFgqY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]

#### Step: CreateVM with a NIC at out-of-range unit 200

Requested:

```
unit=200 key=-1880184701 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `CreateVM failed for nic-unit-research-e19-tkngxy-create-oor-200: A specified parameter was not correct: unitNumber`

Fault: `InvalidArgument`

> A specified parameter was not correct: unitNumber
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEcaG9zdGQAAIKtV2xpYnZtYWNvcmUuc28AADP3OACo7DoAAfdHAe2fZWhvc3RkAAF6p2WBjO98AYFsTH0BgXPDZwGBcM5nAYEs22cBgc/iZwGBAJNiAYHp1W0BgX/kbQGBNNJ0AYFA13QBAahu2IJPhSsBbGlidmltLXR5cGVzLnNvAAOnfSdsaWJ2bW9taS5zbwABOzxuBOsXFWxpYnZhcGktY29yZS5zby4yAAALiSQA0mklAG1sJQDXJlUFd2gIbGliYy5zby42AAWAaRA=[/context]
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwETdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90cBJbctbGlidm1vbWkuc28AAgsdL3ZweGEAApQEHgK8xB0CQv4dAp9wHgLTiR4CwdcdAjnfHgALiSQA0mklAG1sJQDXJlUDd2gIbGliYy5zby42AAOAaRA=[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEudnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBC0CVBIF+WZUEgW7kRQKCpUePAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEmdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdgAcEYIWxpYnZtb21pLnNvAAF/pTiCNdxFAnZweGQAgkdVKwWCHECVBIJ+WZUEgm7kRQKDpUePAWxpYnZpbS10eXBlcy5zbwAB6PMygrwLLwWC9A0vBQEL4CiC1zibAoKBTOAEgvGoLAWCW6osBYL9WysFgpuqLAWC/rQsBYKgOysFgltbKwWCplsrBYKf2CwFgqY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]

#### Step: ReconfigVM_Task Add at out-of-range unit 200

Requested:

```
unit=200 key=-2052801363 controllerKey=0 kind=add VirtualVmxnet3 backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Observed:

```
unit=7 key=4000 controllerKey=100 kind=VirtualVmxnet3 mac=00:50:56:a7:92:74/assigned backing=VirtualEthernetCardNetworkBackingInfo(VM Network)
```

Error: `ReconfigVM_Task failed: A specified parameter was not correct: unitNumber`

Fault: `InvalidArgument`

> A specified parameter was not correct: unitNumber
>
> backtrace: [context]zKq7AVIDAgAAAKD/hwEcdnB4YQAAgq1XbGlidm1hY29yZS5zbwAAM/c4AKjsOgAB90cBErUTbGlidm1vbWkuc28AAVHXLQItbR12cHhhAAK/+B4CT/0eAujqNgLgSC2DMR4rAWxpYnZpbS10eXBlcy5zbwABp30nAqSjIAFDmxsCmc0uApE3HgJC/h0Cn3AeAvyGHgLB1x0COd8eAAuJJADSaSUAbWwlANcmVQR3aAhsaWJjLnNvLjYABIBpEA==[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwExdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQAGHhggUHuogR2cHhkAIHoGKUEgfIwpQSB8agsBYFbqiwFgf1bKwWBm6osBYH+tCwFgaA7KwWBW1srBYGmWysFgVe5LAWBOj2VBIECPpUEgQXZGQSB5dwZBIF73RkEgXMZGgSCDb2OAWxpYnZpbS10eXBlcy5zbwAD6PMybGlidm1vbWkuc28AgbwLLwWB9A0vBQML4CiB1zibAoGBTOAEgfGoLAWBW6osBYH9WysFgZuqLAWB/rQsBYGgOysFgVtbKwWBplsrBYGf2CwFgaY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]
>
> backtrace: [context]zKq7AVEDAQAAAIT/hwEpdnB4ZAAAGdhtbGlidm1hY29yZS5zbwAA/x1TAIM4PwCwyFQABWdgAcEYIWxpYnZtb21pLnNvAAF/pTiCNdxFAnZweGQAgkdVKwWCSz2VBIICPpUEggXZGQSC5dwZBIJ73RkEgnMZGgSDDb2OAWxpYnZpbS10eXBlcy5zbwAB6PMygrwLLwWC9A0vBQEL4CiC1zibAoKBTOAEgvGoLAWCW6osBYL9WysFgpuqLAWC/rQsBYKgOysFgltbKwWCplsrBYKf2CwFgqY+QwIAf8g+AOSPQAACWkEARlxBAHtcQQB/yD4AHYprBJvnCGxpYmMuc28uNgAEeHAQ[/context]

**Findings**:

- Step "CreateVM with a NIC at out-of-range unit 3" succeeded but was silently reassigned: Requested unit 3 is absent from the observed units [7] — the task succeeded but the platform placed the device elsewhere.
- Step "ReconfigVM_Task Add at out-of-range unit 3" succeeded but was silently reassigned: Requested unit 3 is absent from the observed units [7 8] — the task succeeded but the platform placed the device elsewhere.
- Step "CreateVM with a NIC at out-of-range unit 17" returned fault `InvalidArgument` — vSphere rejects this out-of-range value outright.
- Step "ReconfigVM_Task Add at out-of-range unit 17" returned fault `InvalidArgument` — vSphere rejects this out-of-range value outright.
- Step "CreateVM with a NIC at out-of-range unit 200" returned fault `InvalidArgument` — vSphere rejects this out-of-range value outright.
- Step "ReconfigVM_Task Add at out-of-range unit 200" returned fault `InvalidArgument` — vSphere rejects this out-of-range value outright.
