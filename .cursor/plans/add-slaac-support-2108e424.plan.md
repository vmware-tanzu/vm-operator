<!-- 2108e424-1d6b-448d-a518-cdcbd191d6ed 27162d15-5dd6-4b8c-a5a7-170c6c7df52a -->
# Add True SLAAC Support via AutoConfigurationEnabled for IPv6 Networking

## Overview

This plan adds **true SLAAC (Stateless Address Autoconfiguration) support** to VM Operator through the `AutoConfigurationEnabled` field, which matches the existing status field name. Currently, SLAAC is not properly supported - `accept-ra` in netplan is only set when `dhcp6: true`, which is not true SLAAC. This plan enables pure SLAAC configuration (accept-ra: true, dhcp6: false, no static IPv6 addresses) and also includes splitting NetOP's DHCP flag into separate DHCP4 and DHCP6 flags for independent control.

### Current Support Status

**What's Currently Supported:**

- ✅ Static IPv4 addresses
- ✅ Static IPv6 addresses
- ✅ DHCP4 (IPv4 DHCP)
- ✅ DHCP6 (IPv6 DHCP)
- ✅ Dual-stack (IPv4 + IPv6 combinations)
- ✅ `autoConfigurationEnabled` in status (read-only, observed state)

**What's NOT Currently Supported:**

- ❌ True SLAAC (accept-ra: true without dhcp6)
- ❌ Independent DHCP4/DHCP6 control from NetOP
- ❌ SLAAC configuration from NetOP status
- ❌ SLAAC with GOSC (LinuxPrep/Sysprep)

### IPv6 Configuration Flags

| Flag | Scope | Current Support | After This Plan |

|------|-------|----------------|-----------------|

| `dhcp4` | Per-interface | ✅ Supported | ✅ Supported |

| `dhcp6` | Per-interface | ✅ Supported | ✅ Supported |

| `autoConfigurationEnabled` (spec) | Per-interface | ❌ Not supported | ✅ **New: Supported** |

| `autoConfigurationEnabled` (status) | Per-interface | ✅ Read-only | ✅ Read-only (unchanged) |

| NetOP `dhcp4Enabled` | Per-interface | ❌ Not supported | ✅ **New: Supported** |

| NetOP `dhcp6Enabled` | Per-interface | ❌ Not supported | ✅ **New: Supported** |

| NetOP `autoConfigurationEnabled` | Per-interface | ❌ Not supported | ✅ **New: Supported** |

### Valid IPv6 Configuration Combinations

| IPv4 Method | IPv6 Method | Valid? | Notes |

|--------------|-------------|--------|-------|

| Static | Static | ✅ | Both addresses manually configured |

| Static | DHCP6 | ✅ | IPv4 static, IPv6 via DHCP |

| Static | AutoConfigurationEnabled | ✅ | IPv4 static, IPv6 via SLAAC |

| DHCP4 | Static | ✅ | IPv4 via DHCP, IPv6 static |

| DHCP4 | DHCP6 | ✅ | Both via DHCP |

| DHCP4 | AutoConfigurationEnabled | ✅ | IPv4 via DHCP, IPv6 via SLAAC |

| DHCP4 | DHCP6 + AutoConfigurationEnabled | ⚠️ | Can coexist (SLAAC for addresses, DHCP6 for DNS) |

| Static | Static + AutoConfigurationEnabled | ❌ | Mutually exclusive (validation prevents) |

| Static | DHCP6 + AutoConfigurationEnabled | ❌ | Mutually exclusive (validation prevents) |

## Current State

- `accept-ra` in netplan is only set when `dhcp6: true` (not true SLAAC)
- No way to configure pure SLAAC (accept-ra: true, dhcp6: false, no static IPv6 addresses)
- `autoConfigurationEnabled` exists in status but is read-only
- NetOP's `IPAssignmentMode: DHCP` sets both `dhcp4: true` and `dhcp6: true` (no independent control)

## Implementation Plan

### 1. API Changes

Add `AutoConfigurationEnabled` field to `VirtualMachineNetworkInterfaceSpec` in all API versions:

- **Files to modify:**
        - `api/v1alpha2/virtualmachine_network_types.go`
        - `api/v1alpha3/virtualmachine_network_types.go`
        - `api/v1alpha4/virtualmachine_network_types.go`
        - `api/v1alpha5/virtualmachine_network_types.go`

- **Field definition:**
  ```go
  // AutoConfigurationEnabled indicates whether or not this interface uses IPv6
  // Stateless Address Autoconfiguration (SLAAC) for IP6 networking.
  //
  // When enabled, the interface will accept Router Advertisements and automatically
  // configure IPv6 addresses based on the network prefix from the router.
  //
  // This field corresponds to the observed state in status.network.interfaces[].ip.autoConfigurationEnabled.
  //
  // Please note this field is mutually exclusive with IPv6 addresses in the
  // Addresses field, the Gateway6 field, and the DHCP6 field.
  //
  // Please note this feature is available only with the following bootstrap
  // providers: CloudInit. GOSC (LinuxPrep/Sysprep) does not support SLAAC.
  //
  // +optional
  AutoConfigurationEnabled *bool `json:"autoConfigurationEnabled,omitempty"`
  ```


**Note:** Using `*bool` (pointer) instead of `bool` to allow three states: `true`, `false`, and `nil` (unset/use default). This matches the pattern used in the status field and allows for explicit disable.

### 2. Network Processing Logic

Update `applyInterfaceSpecToResult` in `pkg/providers/vsphere/network/network.go`:

- **Location:** Around line 210 (after DHCP6 handling)
- **Change:** Add logic to set `AutoConfigurationEnabled` field in `NetworkInterfaceResult` struct
- **Update struct:** Add `AutoConfigurationEnabled bool` field to `NetworkInterfaceResult` (around line 65)
- **Processing:** 
  ```go
  if interfaceSpec.AutoConfigurationEnabled != nil {
      result.AutoConfigurationEnabled = *interfaceSpec.AutoConfigurationEnabled
  }
  ```


### 3. Netplan Generation

Update `NetPlanCustomization` in `pkg/providers/vsphere/network/netplan.go`:

- **Location:** Line 41 (current: `npEth.AcceptRa = &r.DHCP6`)
- **Change:** Make `accept-ra` independent of `dhcp6` with smart defaults:
  ```go
  // Set accept-ra based on configuration:
  // - AutoConfigurationEnabled requires accept-ra: true
  // - DHCP6 typically uses accept-ra: true (for additional RA info like DNS)
  // - Static IPv6 only: accept-ra: false (no RAs needed)
  // - Otherwise: unset (use OS default)
  var acceptRa *bool
  if r.AutoConfigurationEnabled {
      // AutoConfigurationEnabled (SLAAC) requires router advertisements
      acceptRa = ptr.To(true)
  } else if r.DHCP6 {
      // DHCP6 with RAs is the common case (RAs provide additional info)
      acceptRa = ptr.To(true)
  } else if hasStaticIPv6 {
      // Static IPv6 addresses only - no need for RAs
      acceptRa = ptr.To(false)
  }
  // If acceptRa is nil, netplan will use OS default
  npEth.AcceptRa = acceptRa
  ```


### 4. Validation Webhook

Update `webhooks/virtualmachine/validation/virtualmachine_validator.go`:

- **Location:** After DHCP6 validation (around line 960)
- **Add validation:**
        - `autoConfigurationEnabled` is mutually exclusive with IPv6 addresses in `addresses` field
        - `autoConfigurationEnabled` is mutually exclusive with `gateway6` field
        - `autoConfigurationEnabled` is mutually exclusive with `dhcp6` field

### 5. GOSC Support

**AutoConfigurationEnabled (SLAAC) is NOT supported with GOSC (LinuxPrep/Sysprep).**

vSphere Guest OS Customization (GOSC) API does not support SLAAC. The API only supports:

- `CustomizationFixedIpV6` for static IPv6 addresses
- `CustomizationDhcpIpV6Generator` for DHCPv6

**Implementation:**

- Add a comment in `GuestOSCustomization` function (`pkg/providers/vsphere/network/gosc.go`) noting that AutoConfigurationEnabled is not supported
- When AutoConfigurationEnabled is enabled but GOSC is used, log a warning that it will be ignored
- Document in API field comments and documentation that AutoConfigurationEnabled is only supported with Cloud-Init bootstrap provider

### 6. Net-Operator (NetOP) Integration

**NetOP SHOULD add separate DHCP4, DHCP6, and AutoConfigurationEnabled flags in NetworkInterface status** to provide independent control over IPv4 and IPv6 address assignment methods.

#### Current NetOP Limitation

Currently, NetOP only provides:

- `IPAssignmentMode: DHCP` → VM Operator sets both `dhcp4: true` and `dhcp6: true` (no independent control)
- `IPAssignmentMode: StaticPool` → VM Operator processes static IPConfigs
- `IPAssignmentMode: None` → VM Operator sets `NoIPAM: true`

This doesn't allow for scenarios like:

- DHCP4 + AutoConfigurationEnabled (common dual-stack)
- DHCP4 + Static IPv6
- AutoConfigurationEnabled only (IPv6-only with SLAAC)

#### Proposed NetOP Changes

NetOP should add separate boolean fields in NetworkInterface status:

**New fields in NetworkInterface status:**

```yaml
status:
  dhcp4Enabled: *bool                      # Optional: Enable DHCP for IPv4
  dhcp6Enabled: *bool                      # Optional: Enable DHCP for IPv6
  autoConfigurationEnabled: *bool          # Optional: Enable SLAAC for IPv6
```

**Backward compatibility:**

- Keep `IPAssignmentMode` for backward compatibility
- When `IPAssignmentMode: DHCP` is set → treat as `dhcp4Enabled: true, dhcp6Enabled: true` (if new fields not set)
- When new fields are set → they take precedence over `IPAssignmentMode`

#### VM Operator Implementation

Update `netOpNetIfToResult` in `pkg/providers/vsphere/network/network.go`:

```go
// Process IPAssignmentMode (backward compatibility)
switch netIf.Status.IPAssignmentMode {
case netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP:
    // Only set if new flags not provided (backward compatibility)
    if netIf.Status.DHCP4Enabled == nil && netIf.Status.DHCP6Enabled == nil {
        result.DHCP4 = true
        result.DHCP6 = true
    }
case netopv1alpha1.NetworkInterfaceIPAssignmentModeNone:
    result.NoIPAM = true
default: // StaticPool
    // Process IPConfigs...
}

// Process new separate flags (take precedence over IPAssignmentMode)
if netIf.Status.DHCP4Enabled != nil {
    result.DHCP4 = *netIf.Status.DHCP4Enabled
}
if netIf.Status.DHCP6Enabled != nil {
    result.DHCP6 = *netIf.Status.DHCP6Enabled
}
if netIf.Status.AutoConfigurationEnabled != nil {
    result.AutoConfigurationEnabled = *netIf.Status.AutoConfigurationEnabled
}
```

#### Priority/Override Logic

1. **NetOP provides flags** → VM Operator sets corresponding flags in result (default from network)
2. **User can override** → User sets flags in interface spec override NetOP's settings
3. **Backward compatibility** → `IPAssignmentMode: DHCP` still works if new fields not set

### 7. Documentation Updates

- **File:** `docs/concepts/services-networking/dual-stack-networking.md`
        - Add section explaining AutoConfigurationEnabled (SLAAC)
        - Document mutually exclusive rules
        - Add examples of AutoConfigurationEnabled configurations
        - Note Cloud-Init only limitation
        - Document NetOP's new flags

- **File:** `docs/examples/ipv6-dhcp-configurations.yaml` (or create new file)
        - Add AutoConfigurationEnabled configuration examples
        - Add examples showing NetOP flags

### 8. Unit Tests

Add tests in:

- **`pkg/providers/vsphere/network/network_test.go`:**
        - Test AutoConfigurationEnabled flag processing
        - Test AutoConfigurationEnabled with various combinations
        - Test mutually exclusive validation
        - Test NetOP's new `dhcp4Enabled`, `dhcp6Enabled`, `autoConfigurationEnabled` flags
        - Test backward compatibility with `IPAssignmentMode: DHCP`

- **`pkg/providers/vsphere/network/netplan_test.go`:**
        - Test `accept-ra` is set correctly for AutoConfigurationEnabled
        - Test AutoConfigurationEnabled without DHCP6
        - Test AutoConfigurationEnabled with DHCP4 (dual-stack)

- **`webhooks/virtualmachine/validation/virtualmachine_validator_unit_test.go`:**
        - Test AutoConfigurationEnabled validation rules
        - Test mutually exclusive scenarios

### 9. CRD Generation

After API changes, regenerate CRDs:

- Run `make generate` to update CRD YAML files
- Verify new `autoConfigurationEnabled` field appears in CRD schema

## Example Usage

```yaml
apiVersion: vmoperator.vmware.com/v1alpha5
kind: VirtualMachine
spec:
  bootstrap:
    cloudInit:  # AutoConfigurationEnabled requires Cloud-Init
      cloudConfig:
        users:
    - name: default
  network:
    interfaces:
  - name: eth0
      network:
        name: my-network
      autoConfigurationEnabled: true  # Enable SLAAC for IPv6 (Cloud-Init only)
      dhcp4: true                     # Can combine with DHCP4 for dual-stack
```

## NetOP Status Example

```yaml
apiVersion: netoperator.vmware.com/v1alpha1
kind: NetworkInterface
status:
  ipAssignmentMode: StaticPool           # Backward compatible
  dhcp4Enabled: true                     # New: Independent DHCP4 control
  autoConfigurationEnabled: true         # New: SLAAC for IPv6
  ipConfigs: []                          # Empty when using DHCP/SLAAC
```

## Testing Strategy

1. **Unit tests:** Cover all validation and processing logic
2. **Integration tests:** Test AutoConfigurationEnabled with Cloud-Init bootstrap
3. **Negative tests:** Verify GOSC ignores AutoConfigurationEnabled (with warning)
4. **Validation tests:** Test all mutually exclusive scenarios
5. **NetOP integration tests:** Test reading new flags from NetOP status
6. **Backward compatibility tests:** Verify `IPAssignmentMode: DHCP` still works

## Migration Notes

- New field is optional and defaults to `nil` (unset)
- No breaking changes to existing configurations
- Existing VMs continue to work without modification
- NetOP's new flags are optional - backward compatible
- `IPAssignmentMode: DHCP` continues to work if new flags not set

## Dependencies

- **NetOP changes required**: NetOP must add `dhcp4Enabled`, `dhcp6Enabled`, and `autoConfigurationEnabled` fields to NetworkInterface status
- **VM Operator changes**: Read and process new NetOP flags (with backward compatibility)

### To-dos

- [ ] Add AutoConfigurationEnabled field to VirtualMachineNetworkInterfaceSpec in all API versions (v1alpha2-v1alpha5)
- [ ] Add AutoConfigurationEnabled bool field to NetworkInterfaceResult struct in network.go
- [ ] Update applyInterfaceSpecToResult to process AutoConfigurationEnabled flag from interface spec
- [ ] Update NetPlanCustomization to set accept-ra based on AutoConfigurationEnabled flag (independent of DHCP6)
- [ ] Add validation rules for AutoConfigurationEnabled: mutually exclusive with IPv6 addresses, gateway6, and dhcp6
- [ ] Add comment in GuestOSCustomization noting AutoConfigurationEnabled is not supported, log warning when used with GOSC
- [ ] Update netOpNetIfToResult to read new NetOP flags: dhcp4Enabled, dhcp6Enabled, autoConfigurationEnabled (with backward compatibility for IPAssignmentMode)
- [ ] Add unit tests for AutoConfigurationEnabled processing, netplan generation, validation, and NetOP flag reading
- [ ] Update documentation with AutoConfigurationEnabled examples, limitations, mutually exclusive rules, and NetOP flag documentation
- [ ] Regenerate CRDs after API changes using make generate