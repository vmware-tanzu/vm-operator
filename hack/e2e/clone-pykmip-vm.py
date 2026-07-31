#!/usr/bin/env python3
"""
clone-pykmip-vm.py

Clones the testbed gateway VM into a lightweight dedicated PyKMIP appliance:
  1. Find the gateway VM by standard testbed naming convention.
  2. Clone it (powered-off) onto the same ESXi host with 2 vCPUs / 2 GiB RAM,
     pinning placement so stretched-supervisor clones stay on the right site.
  3. Remove every NIC except the management NIC (identified by portgroup name)
     to avoid overlay/NSX IP conflicts on multi-NIC gateway VMs.
  4. Rewrite guestinfo so cloud-init treats the clone as a new instance and
     systemd-networkd acquires a DHCP lease on the surviving NIC.
  5. Power the clone on.
  6. Wait for a management-network IP (filtered by MGMT_CIDR).
  7. Print the IP to stdout (all progress goes to stderr).

Idempotent: if a VM named PYKMIP_VM_NAME already exists its IP is returned
without re-cloning.

Environment (exported by setup-testbed-env.sh --e2e):
  VC_URL            vCenter IP or hostname
  VC_VIM_USERNAME   vSphere API username  (e.g. administrator@vsphere.local)
  VC_VIM_PASSWORD   vSphere API password

Optional:
  PYKMIP_VM_NAME    clone name            (default: gce2e-pykmip)
  PYKMIP_CPUS       vCPUs for clone       (default: 2)
  PYKMIP_MEM_MB     RAM in MiB            (default: 2048)
  MGMT_CIDR         management CIDR       (default: 10.0.0.0/8)
  PYKMIP_IP_TIMEOUT seconds to wait for IP (default: 300)
"""

from __future__ import annotations

import ipaddress
import os
import ssl
import sys
import time
import traceback
from base64 import b64encode

from pyVim.connect import SmartConnect, Disconnect
from pyVmomi import vim

sys.path.insert(0, os.path.dirname(__file__))
from lib.vm import CloneVM, PowerOnVM, ReconfigureVM

# ── configuration from environment ───────────────────────────────────────────

VC_URL            = os.environ.get("VC_URL", "")
VC_VIM_USERNAME   = os.environ.get("VC_VIM_USERNAME", "")
VC_VIM_PASSWORD   = os.environ.get("VC_VIM_PASSWORD", "")
PYKMIP_VM_NAME    = os.environ.get("PYKMIP_VM_NAME", "gce2e-pykmip")
PYKMIP_CPUS       = int(os.environ.get("PYKMIP_CPUS", "2"))
PYKMIP_MEM_MB     = int(os.environ.get("PYKMIP_MEM_MB", "2048"))
MGMT_CIDR         = os.environ.get("MGMT_CIDR", "10.0.0.0/8")
PYKMIP_IP_TIMEOUT = int(os.environ.get("PYKMIP_IP_TIMEOUT", "300"))

# ── gateway VM name prefixes — same convention as proxy.sh / kms.sh ──────────
_GATEWAY_PREFIXES = ("external-gateway", "external-vm-gateway")


def _log(msg: str) -> None:
    print(f"[clone-pykmip-vm] {msg}", file=sys.stderr, flush=True)


def _connect_vc():
    if not all([VC_URL, VC_VIM_USERNAME, VC_VIM_PASSWORD]):
        raise EnvironmentError(
            "VC_URL, VC_VIM_USERNAME, and VC_VIM_PASSWORD must all be set"
        )
    ctx = ssl._create_unverified_context()
    return SmartConnect(
        host=VC_URL,
        user=VC_VIM_USERNAME,
        pwd=VC_VIM_PASSWORD,
        sslContext=ctx,
    )


def _iter_vms(si):
    """Yield every VirtualMachine managed object in the inventory."""
    content = si.RetrieveContent()
    view = content.viewManager.CreateContainerView(
        content.rootFolder, [vim.VirtualMachine], True
    )
    try:
        yield from view.view
    finally:
        view.Destroy()


def _find_vm_by_name(si, name: str):
    for vm in _iter_vms(si):
        if vm.name == name:
            return vm
    return None


def _find_gateway_vm(si):
    """Return the first gateway VM matching the testbed naming convention."""
    for vm in _iter_vms(si):
        for prefix in _GATEWAY_PREFIXES:
            if vm.name.startswith(prefix):
                _log(f"Found gateway VM: {vm.name}")
                return vm
    raise LookupError(
        f"No gateway VM found with prefixes {_GATEWAY_PREFIXES}. "
        "Is this a VDS or NSX testbed with a gateway VM?"
    )


def _build_relocate_spec(src_vm) -> vim.vm.RelocateSpec:
    """Pin clone to the same ESXi host, datastore, and resource pool as source.

    Pinning the host prevents stretched-supervisor DRS from placing the clone
    on a remote-site host whose management network is unreachable from vCenter.
    """
    spec = vim.vm.RelocateSpec()
    spec.host = src_vm.runtime.host
    spec.pool = src_vm.resourcePool
    if src_vm.datastore:
        spec.datastore = src_vm.datastore[0]
    return spec


def _find_mgmt_nic(vm):
    """Return the NIC connected to the management (DHCP) portgroup.

    On VDS/Photon gateway VMs 'Network adapter 1' is eth0 = PRIMARY_NETWORK
    (static 192.168.1.1), while the management NIC with DHCP lives on a
    different adapter.  We identify the management NIC by its portgroup name
    rather than its adapter number so we keep the right one regardless of
    adapter ordering.
    """
    mgmt_keywords = ("vm network", "management", "mgmt")
    nics = [
        d for d in vm.config.hardware.device
        if isinstance(d, vim.vm.device.VirtualEthernetCard)
    ]
    for nic in nics:
        backing = nic.backing
        pg_name = ""
        if hasattr(backing, "network") and backing.network:
            pg_name = backing.network.name.lower()
        elif hasattr(backing, "deviceName"):
            pg_name = backing.deviceName.lower()
        elif hasattr(backing, "port") and hasattr(backing.port, "portgroupKey"):
            pg_name = (nic.deviceInfo.summary or "").lower()
        if any(kw in pg_name for kw in mgmt_keywords):
            _log(f"Management NIC identified: {nic.deviceInfo.label} (portgroup: {pg_name})")
            return nic
    # Fallback: return the first NIC (original behaviour for NSX/Ubuntu).
    _log("No management portgroup match — falling back to first NIC")
    return nics[0] if nics else None


def _remove_extra_nics(vm):
    """Remove all NICs except the management NIC from the powered-off clone.

    On VDS/Photon testbeds the management NIC (DHCP) is not always 'Network
    adapter 1' — e.g. eth0 carries a static primary-network IP while eth1 is
    the DHCP management interface.  We identify the keeper by its portgroup
    name rather than adapter number and return it so _fix_network can use its
    MAC.
    """
    mgmt_nic = _find_mgmt_nic(vm)
    if mgmt_nic is None:
        _log("No NICs found — nothing to remove")
        return None

    devices_to_remove = [
        vim.vm.device.VirtualDeviceSpec(
            operation=vim.vm.device.VirtualDeviceSpec.Operation.remove,
            device=dev,
        )
        for dev in vm.config.hardware.device
        if isinstance(dev, vim.vm.device.VirtualEthernetCard)
        and dev.key != mgmt_nic.key
    ]
    if not devices_to_remove:
        _log("Only one NIC present — no extras to remove")
        return mgmt_nic
    _log(f"Removing {len(devices_to_remove)} extra NIC(s) (keeping '{mgmt_nic.deviceInfo.label}')")
    ReconfigureVM(vm, vim.vm.ConfigSpec(deviceChange=devices_to_remove))
    return mgmt_nic


def _fix_cloud_init_metadata(vm, mgmt_nic) -> None:
    """Rewrite guestinfo so the clone acquires a DHCP lease on the management NIC.

    Three things are required:
    1. Fresh instance-id — without it cloud-init finds the gateway's cached
       state in /var/lib/cloud/ and skips all modules including network.
    2. Correct MAC in metadata — CloneVM assigns a new MAC; the inherited
       metadata still references the gateway's original MAC.
    3. Systemd-networkd fix (Photon OS) — the gateway's cloned disk has
       /etc/systemd/network/ files that either carry static IPs or match the
       old MAC.  A userdata script replaces them with a single MAC-matched
       DHCP file so the first-boot DHCP request succeeds.
    """
    new_mac = mgmt_nic.macAddress
    _log(f"Rewriting cloud-init metadata for MAC {new_mac}")

    instance_id = f"pykmip-{new_mac.replace(':', '')}"
    encoded_md = b64encode(
        f"instance-id: {instance_id}\nlocal-hostname: pykmip-server\n".encode()
    ).decode()

    # Photon OS uses systemd-networkd, not netplan.  Replace all gateway
    # network unit files with a single MAC-matched DHCP config so that
    # systemd-networkd brings up the management NIC on first boot.
    userdata = b64encode(f"""\
#!/bin/bash
find /etc/systemd/network -name '*.network' -delete
cat > /etc/systemd/network/10-mgmt-dhcp.network << 'EOF'
[Match]
MACAddress={new_mac}

[Network]
DHCP=yes
IPv6AcceptRA=no

[DHCPv4]
SendRelease=no
EOF
systemctl restart systemd-networkd
""".encode()).decode()

    ReconfigureVM(vm, vim.vm.ConfigSpec(extraConfig=[
        vim.option.OptionValue(key="guestinfo.metadata",          value=encoded_md),
        vim.option.OptionValue(key="guestinfo.metadata.encoding", value="base64"),
        vim.option.OptionValue(key="guestinfo.userdata",          value=userdata),
        vim.option.OptionValue(key="guestinfo.userdata.encoding", value="base64"),
    ]))


def _wait_for_mgmt_ip(vm, cidr: str, timeout: int) -> str:
    """Poll vm.guest until an IP inside *cidr* is reported by VMware Tools.

    Returns the first matching IP address string.
    """
    network = ipaddress.ip_network(cidr, strict=False)
    _log(f"Waiting up to {timeout}s for a management IP in {cidr}...")
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        vm.Reload()
        guest = vm.guest
        candidates: list[str] = []
        if guest and guest.ipAddress:
            candidates.append(guest.ipAddress)
        if guest and guest.net:
            for nic in guest.net:
                if nic.ipConfig:
                    for addr in nic.ipConfig.ipAddress:
                        candidates.append(addr.ipAddress)
        for ip_str in candidates:
            try:
                if ipaddress.ip_address(ip_str) in network:
                    return ip_str
            except ValueError:
                continue
        time.sleep(5)
    raise TimeoutError(
        f"VM '{vm.name}' did not receive a management IP in {cidr} within {timeout}s"
    )


def main() -> None:
    si = _connect_vc()
    _log(f"Connected to vCenter {VC_URL}")
    try:
        # Idempotency: if the clone already exists return its IP without re-cloning.
        existing = _find_vm_by_name(si, PYKMIP_VM_NAME)
        if existing is not None:
            _log(f"VM '{PYKMIP_VM_NAME}' already exists — skipping clone")
            if existing.runtime.powerState != "poweredOn":
                _log("VM is not powered on — powering on...")
                PowerOnVM(existing)
            ip = _wait_for_mgmt_ip(existing, MGMT_CIDR, PYKMIP_IP_TIMEOUT)
            print(ip)
            return

        gateway_vm = _find_gateway_vm(si)

        relocate_spec = _build_relocate_spec(gateway_vm)
        config_spec = vim.vm.ConfigSpec(numCPUs=PYKMIP_CPUS, memoryMB=PYKMIP_MEM_MB)

        _log(
            f"Cloning '{gateway_vm.name}' → '{PYKMIP_VM_NAME}' "
            f"({PYKMIP_CPUS} vCPU / {PYKMIP_MEM_MB} MiB, powered-off) ..."
        )
        clone_vm = CloneVM(
            gateway_vm,
            PYKMIP_VM_NAME,
            relocate_spec,
            isPowerOn=False,
            configSpec=config_spec,
        )
        _log(f"Clone created: {clone_vm.name}")

        # Remove extra NICs before power-on; returns the surviving management NIC.
        mgmt_nic = _remove_extra_nics(clone_vm)

        # Rewrite cloud-init metadata and systemd-networkd config for the
        # management NIC's new MAC so DHCP fires on first boot.
        _fix_cloud_init_metadata(clone_vm, mgmt_nic)

        _log("Powering on...")
        PowerOnVM(clone_vm)

        ip = _wait_for_mgmt_ip(clone_vm, MGMT_CIDR, PYKMIP_IP_TIMEOUT)
        _log(f"Management IP assigned: {ip}")
        print(ip)
    finally:
        Disconnect(si)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"[clone-pykmip-vm] ERROR: {exc}", file=sys.stderr)
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
