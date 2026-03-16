#!/usr/bin/env python3
"""
new-schema-version.py creates a new API schema version for VM Operator by
copying the most recent version and updating all references. It uses the
conventions established in PR vmware-tanzu/vm-operator#1098 (Introduce
v1alpha5).

Requires Python 3.9 or later.

Usage:
    hack/new-schema-version.py <new-version>

Example:
    hack/new-schema-version.py v1alpha6

The script uses the most recent API version (by Kubernetes version ordering;
see https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/
custom-resource-definition-versioning/) as the basis for the new version.
"""

from __future__ import annotations

import argparse
import os
import re
import shutil
import subprocess
import sys
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable

# Spinner and status display (used in run() and _run_make())
SPINNER_CHARS = "|/-\\"
SPINNER_BRIGHT_WHITE = "\033[97m"
SPINNER_RESET = "\033[0m"
STATUS_OK = "\033[92m[OK]\033[0m"
STATUS_FAIL = "\033[91m[FAIL]\033[0m"

# -----------------------------------------------------------------------------
# Version Handling
# -----------------------------------------------------------------------------


@dataclass(frozen=True)
class KubeVersion:
    """Represents a Kubernetes API version (e.g., v1, v1alpha1, v1beta2)."""

    major: int
    stability: str  # '', 'alpha', or 'beta'
    patch: int
    raw: str

    # Stability ordering: stable > beta > alpha
    STABILITY_ORDER = {"": 2, "beta": 1, "alpha": 0}

    @classmethod
    def parse(cls, version: str) -> KubeVersion | None:
        """Parse a version string into a KubeVersion object."""
        match = re.match(r"^v(\d+)(alpha|beta)?(\d*)$", version)
        if not match:
            return None
        major = int(match.group(1))
        stability = match.group(2) or ""
        patch = int(match.group(3)) if match.group(3) else 0
        return cls(major=major, stability=stability, patch=patch, raw=version)

    @classmethod
    def validate(cls, version: str) -> bool:
        """Check if a version string is valid."""
        return cls.parse(version) is not None

    def __lt__(self, other: KubeVersion) -> bool:
        """Compare versions using Kubernetes version ordering."""
        if self.major != other.major:
            return self.major < other.major
        self_stability = self.STABILITY_ORDER.get(self.stability, -1)
        other_stability = self.STABILITY_ORDER.get(other.stability, -1)
        if self_stability != other_stability:
            return self_stability < other_stability
        return self.patch < other.patch

    def __str__(self) -> str:
        return self.raw


def sort_versions(versions: list[str]) -> list[str]:
    """Sort version strings using Kubernetes version ordering."""
    parsed = [(v, KubeVersion.parse(v)) for v in versions]
    valid = [(v, kv) for v, kv in parsed if kv is not None]
    valid.sort(key=lambda x: x[1])
    return [v for v, _ in valid]


# -----------------------------------------------------------------------------
# File Operations
# -----------------------------------------------------------------------------


class FileOps:
    """File operations with logging and dry-run support."""

    def __init__(self, root: Path, dry_run: bool = False, verbose: bool = False):
        self.root = root
        self.dry_run = dry_run
        self.verbose = verbose

    def log(self, msg: str) -> None:
        """Log a message if verbose mode is enabled."""
        if self.verbose:
            print(f"  {msg}")

    def copy_tree(self, src: Path, dst: Path) -> None:
        """Copy a directory tree."""
        self.log(f"copy {src} -> {dst}")
        if not self.dry_run:
            shutil.copytree(src, dst)

    def copy_file(self, src: Path, dst: Path) -> None:
        """Copy a single file."""
        self.log(f"copy {src} -> {dst}")
        if not self.dry_run:
            shutil.copy2(src, dst)

    def move(self, src: Path, dst: Path) -> None:
        """Move a file or directory."""
        self.log(f"move {src} -> {dst}")
        if not self.dry_run:
            shutil.move(str(src), str(dst))

    def mkdir(self, path: Path) -> None:
        """Create a directory (with parents)."""
        self.log(f"mkdir {path}")
        if not self.dry_run:
            path.mkdir(parents=True, exist_ok=True)

    def write_file(self, path: Path, content: str) -> None:
        """Write content to a file."""
        self.log(f"write {path}")
        if not self.dry_run:
            path.write_text(content)

    def read_file(self, path: Path) -> str:
        """Read content from a file."""
        return path.read_text()

    def replace_in_file(
        self, path: Path, old: str, new: str, all_occurrences: bool = True
    ) -> bool:
        """Replace text in a file. Returns True if any replacement was made."""
        content = self.read_file(path)
        if old not in content:
            return False
        if all_occurrences:
            new_content = content.replace(old, new)
        else:
            new_content = content.replace(old, new, 1)
        if content != new_content:
            self.log(f"replace '{old}' -> '{new}' in {path}")
            self.write_file(path, new_content)
            return True
        return False

    def replace_in_tree(
        self,
        directory: Path,
        old: str,
        new: str,
        file_filter: Callable[[Path], bool] | None = None,
    ) -> int:
        """Replace text in all files under a directory.

        Returns count of modified files.
        """
        count = 0
        for path in directory.rglob("*"):
            if path.is_file() and ".git" not in path.parts:
                if file_filter and not file_filter(path):
                    continue
                if self.replace_in_file(path, old, new):
                    count += 1
        return count

    def transform_file(self, path: Path, transformer: Callable[[str], str]) -> bool:
        """Apply a transformation function to file content."""
        content = self.read_file(path)
        new_content = transformer(content)
        if content != new_content:
            self.log(f"transform {path}")
            self.write_file(path, new_content)
            return True
        return False

    def remove_lines_matching(self, path: Path, pattern: str) -> bool:
        """Remove lines matching a regex pattern from a file."""
        content = self.read_file(path)
        lines = content.splitlines(keepends=True)
        regex = re.compile(pattern)
        new_lines = [line for line in lines if not regex.search(line)]
        new_content = "".join(new_lines)
        if content != new_content:
            self.log(f"remove lines matching '{pattern}' from {path}")
            self.write_file(path, new_content)
            return True
        return False

    def find_files(self, directory: Path, pattern: str) -> list[Path]:
        """Find files matching a glob pattern."""
        return list(directory.glob(pattern))


# -----------------------------------------------------------------------------
# Schema Version Migration
# -----------------------------------------------------------------------------


@dataclass
class MigrationContext:
    """Context for the schema version migration."""

    root: Path
    old_ver: str
    new_ver: str
    ops: FileOps

    # Computed properties
    old_ver_alias: str = field(init=False)
    prev_ver: str | None = field(init=False)
    all_versions: list[str] = field(init=False)

    def __post_init__(self):
        self.all_versions = self._discover_api_versions()
        self.old_ver_alias = self._compute_version_alias(self.old_ver)
        self.prev_ver = self._find_previous_version(self.old_ver)

    def _discover_api_versions(self) -> list[str]:
        """Discover all API versions under api/."""
        api_dir = self.root / "api"
        versions = [
            d.name
            for d in api_dir.iterdir()
            if d.is_dir() and d.name.startswith("v") and KubeVersion.validate(d.name)
        ]
        return sort_versions(versions)

    def _compute_version_alias(self, version: str) -> str:
        """Compute import alias for a version (e.g. v1alpha5 -> vmopv1a5)."""
        match = re.match(r"^v1alpha(\d+)$", version)
        if match:
            return f"vmopv1a{match.group(1)}"
        match = re.match(r"^v1beta(\d+)$", version)
        if match:
            return f"vmopv1b{match.group(1)}"
        return "vmopv1old"

    def _find_previous_version(self, version: str) -> str | None:
        """Find the version immediately before the given version."""
        try:
            idx = self.all_versions.index(version)
            return self.all_versions[idx - 1] if idx > 0 else None
        except ValueError:
            return None

    def version_alias(self, version: str) -> str:
        """Get the import alias for any version."""
        return self._compute_version_alias(version)

    @property
    def api_dir(self) -> Path:
        return self.root / "api"

    @property
    def old_api_dir(self) -> Path:
        return self.api_dir / self.old_ver

    @property
    def new_api_dir(self) -> Path:
        return self.api_dir / self.new_ver

    def older_versions(self) -> list[str]:
        """Get versions older than old_ver (excluding old_ver and new_ver)."""
        result = []
        for v in self.all_versions:
            if v == self.old_ver or v == self.new_ver:
                continue
            kv = KubeVersion.parse(v)
            kv_old = KubeVersion.parse(self.old_ver)
            if kv and kv_old and kv < kv_old:
                result.append(v)
        return result


def _discover_root_types(api_dir: Path) -> list[str]:
    """Discover types with // +kubebuilder:object:root=true in api_dir, excluding *List."""
    root_marker = "// +kubebuilder:object:root=true"
    type_re = re.compile(r"^type\s+(\w+)\s+(?:struct|=)", re.MULTILINE)
    found: set[str] = set()
    for go_file in sorted(api_dir.rglob("*.go")):
        content = go_file.read_text(encoding="utf-8")
        for part in content.split(root_marker)[1:]:
            m = type_re.search(part)
            if m:
                name = m.group(1)
                if not name.endswith("List"):
                    found.add(name)
    return sorted(found)


def _generate_conversion_webhooks_go(
    package_name: str,
    api_import_path: str,
    root_types: list[str],
    import_alias: str = "vmopv1",
) -> str:
    """Generate webhooks/conversion/VERSION/webhooks.go that registers conversion for each root type."""
    lines = [
        "// © Broadcom. All Rights Reserved.",
        '// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.',
        "// SPDX-License-Identifier: Apache-2.0",
        "",
        f"package {package_name}",
        "",
        "import (",
        '\tctrl "sigs.k8s.io/controller-runtime"',
        '\tctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"',
        "",
        f'\t{import_alias} "{api_import_path}"',
        '\tpkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"',
        ")",
        "",
        "func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {",
        "",
    ]
    for t in root_types:
        lines.extend(
            [
                f"\tif err := ctrl.NewWebhookManagedBy(mgr).",
                f"\t\tFor(&{import_alias}.{t}{{}}).",
                "\t\tComplete(); err != nil {",
                "",
                "\t\treturn err",
                "\t}",
                "",
            ]
        )
    lines.append("\treturn nil")
    lines.append("}")
    lines.append("")
    return "\n".join(lines)


def _make_v1aN_template_functions_block(new_num: str, new_ver_go: str) -> str:
    """Generate Go source for func v1a{N}TemplateFunctions (vmopv1 types, constants.{VerGo})."""
    return (
        f"func v1a{new_num}TemplateFunctions(\n"
        f"\tnetworkStatusV1A{new_num} vmopv1.NetworkStatus,\n"
        f"\tnetworkDevicesStatusV1A{new_num} []vmopv1.NetworkDeviceStatus) map[string]any {{\n\n"
        "	// Get the first IP address from the first NIC.\n"
        f"	v1alpha{new_num}FirstIP := func() (string, error) {{\n"
        f"		if len(networkDevicesStatusV1A{new_num}) == 0 {{\n"
        '			return "", errors.New("no available network device, check with VI admin")\n'
        "		}\n"
        f"		return networkDevicesStatusV1A{new_num}[0].IPAddresses[0], nil\n"
        "	}\n\n"
        "	// Get the first NIC's MAC address.\n"
        f"	v1alpha{new_num}FirstNicMacAddr := func() (string, error) {{\n"
        f"		if len(networkDevicesStatusV1A{new_num}) == 0 {{\n"
        '			return "", errors.New("no available network device, check with VI admin")\n'
        "		}\n"
        f"		return networkDevicesStatusV1A{new_num}[0].MacAddress, nil\n"
        "	}\n\n"
        "	// Get the first IP address from the ith NIC.\n"
        f"	v1alpha{new_num}FirstIPFromNIC := func(index int) (string, error) {{\n"
        f"		if len(networkDevicesStatusV1A{new_num}) == 0 {{\n"
        '			return "", errors.New("no available network device, check with VI admin")\n'
        "		}\n"
        f"		if index >= len(networkDevicesStatusV1A{new_num}) {{\n"
        '			return "", errors.New("index out of bound")\n'
        "		}\n"
        f"		return networkDevicesStatusV1A{new_num}[index].IPAddresses[0], nil\n"
        "	}\n\n"
        "	// Get all IP addresses from the ith NIC.\n"
        f"	v1alpha{new_num}IPsFromNIC := func(index int) ([]string, error) {{\n"
        f"		if len(networkDevicesStatusV1A{new_num}) == 0 {{\n"
        '			return []string{""}, errors.New("no available network device, check with VI admin")\n'
        "		}\n"
        f"		if index >= len(networkDevicesStatusV1A{new_num}) {{\n"
        '			return []string{""}, errors.New("index out of bound")\n'
        "		}\n"
        f"		return networkDevicesStatusV1A{new_num}[index].IPAddresses, nil\n"
        "	}\n\n"
        "	// Format the first occurred count of nameservers with specific delimiter\n"
        f"	v1alpha{new_num}FormatNameservers := func(count int, delimiter string) (string, error) {{\n"
        f"		var nameservers []string\n"
        f"		if len(networkStatusV1A{new_num}.Nameservers) == 0 {{\n"
        '			return "", errors.New("no available nameservers, check with VI admin")\n'
        "		}\n"
        f"		if count < 0 || count >= len(networkStatusV1A{new_num}.Nameservers) {{\n"
        f"			nameservers = networkStatusV1A{new_num}.Nameservers\n"
        "			return strings.Join(nameservers, delimiter), nil\n"
        "		}\n"
        f"		nameservers = networkStatusV1A{new_num}.Nameservers[:count]\n"
        "		return strings.Join(nameservers, delimiter), nil\n"
        "	}\n\n"
        "	// Get subnet mask from a CIDR notation IP address and prefix length\n"
        f"	v1alpha{new_num}SubnetMask := func(cidr string) (string, error) {{\n"
        "		_, ipv4Net, err := net.ParseCIDR(cidr)\n"
        "		if err != nil {\n"
        '			return "", err\n'
        "		}\n"
        '		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])\n'
        "		return netmask, nil\n"
        "	}\n\n"
        "	// Format an IP address with default netmask CIDR\n"
        f"	v1alpha{new_num}IP := func(IP string) (string, error) {{\n"
        "		if net.ParseIP(IP) == nil {\n"
        '			return "", errors.New("input IP address not valid")\n'
        "		}\n"
        "		defaultMask := net.ParseIP(IP).DefaultMask()\n"
        "		ones, _ := defaultMask.Size()\n"
        '		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)\n'
        "		return expectedCidrNotation, nil\n"
        "	}\n\n"
        "	// Format an IP address with network length(eg. /24) or decimal\n"
        f"	v1alpha{new_num}FormatIP := func(s string, mask string) (string, error) {{\n"
        "		ip, _, err := net.ParseCIDR(s)\n"
        "		if err != nil {\n"
        "			ip = net.ParseIP(s)\n"
        "			if ip == nil {\n"
        '				return "", fmt.Errorf("input IP address not valid")\n'
        "			}\n"
        "		}\n"
        "		s = ip.String()\n"
        '		if mask == "" {\n'
        "			return s, nil\n"
        "		}\n"
        '		if strings.HasPrefix(mask, "/") {\n'
        "			s += mask\n"
        "			if _, _, err := net.ParseCIDR(s); err != nil {\n"
        '				return "", err\n'
        "			}\n"
        "			return s, nil\n"
        "		}\n"
        "		maskIP := net.ParseIP(mask)\n"
        "		if maskIP == nil {\n"
        '			return "", fmt.Errorf("mask is an invalid IP")\n'
        "		}\n"
        "		maskIPBytes := maskIP.To4()\n"
        "		if len(maskIPBytes) == 0 {\n"
        "			maskIPBytes = maskIP.To16()\n"
        "		}\n"
        "		ipNet := net.IPNet{IP: ip, Mask: net.IPMask(maskIPBytes)}\n"
        "		s = ipNet.String()\n"
        "		if _, _, err := net.ParseCIDR(s); err != nil {\n"
        '			return "", fmt.Errorf("invalid ip net: %s", s)\n'
        "		}\n"
        "		return s, nil\n"
        "	}\n\n"
        "	return template.FuncMap{\n"
        f"\t\tconstants.{new_ver_go}FirstIP:           v1alpha{new_num}FirstIP,\n"
        f"\t\tconstants.{new_ver_go}FirstNicMacAddr:   v1alpha{new_num}FirstNicMacAddr,\n"
        f"\t\tconstants.{new_ver_go}FirstIPFromNIC:    v1alpha{new_num}FirstIPFromNIC,\n"
        f"\t\tconstants.{new_ver_go}IPsFromNIC:        v1alpha{new_num}IPsFromNIC,\n"
        f"\t\tconstants.{new_ver_go}FormatNameservers: v1alpha{new_num}FormatNameservers,\n"
        f"\t\tconstants.{new_ver_go}SubnetMask: v1alpha{new_num}SubnetMask,\n"
        f"\t\tconstants.{new_ver_go}IP:         v1alpha{new_num}IP,\n"
        f"\t\tconstants.{new_ver_go}FormatIP:   v1alpha{new_num}FormatIP,\n"
        "\t}\n"
        "}\n"
    )


class SchemaMigration:
    """Handles the migration to a new API schema version."""

    def __init__(self, ctx: MigrationContext):
        self.ctx = ctx
        self.ops = ctx.ops

    def run(self) -> None:
        """Execute all migration steps."""
        steps = [
            (self.step_copy_api_version, "Copying api/{old} to api/{new}"),
            (self.step_demote_old_hub, "Demoting api/{old} from hub to spoke"),
            (
                self.step_remove_storage_version,
                "Removing storage version from api/{old}",
            ),
            (
                self.step_add_local_scheme_builder,
                "Adding localSchemeBuilder to api/{old}",
            ),
            (self.step_update_apis_go, "Updating api/apis.go"),
            (
                self.step_update_doc_conversion_gen,
                "Updating doc.go conversion-gen targets",
            ),
            (
                self.step_create_conversion_subdirs,
                "Creating api/{old} conversion subdirs",
            ),
            (self.step_update_makefile, "Updating Makefile"),
            (
                self.step_create_webhooks_conversion,
                "Creating webhooks/conversion/{new}",
            ),
            (
                self.step_update_older_api_versions,
                "Updating older API versions to reference {new}",
            ),
            (
                self.step_update_webhook_annotations,
                "Updating webhook schema version to {new}",
            ),
            (self.step_update_imports, "Updating imports to api/{new}"),
            (
                self.step_update_vmgroup_test_panic_message,
                "Updating unit and integration tests",
            ),
            (
                self.step_update_golangci_importas,
                "Updating .golangci.yml importas for new hub",
            ),
            (
                self.step_fix_old_ver_import_aliases,
                "Updating api/{old} to use versioned import aliases",
            ),
            (
                self.step_update_template_constants,
                "Adding {new} template constants to pkg/providers/vsphere/constants",
            ),
            (
                self.step_update_bootstrap_templatedata,
                "Demoting hub in bootstrap_templatedata and adding {new} hub",
            ),
            (self.step_run_make_generate, "Running make generate"),
            (
                self.step_run_make_generate_go_conversions,
                "Running make generate-go-conversions",
            ),
            (self.step_run_make_manager_only, "Running make manager-only"),
            (self.step_run_make_lint_go_full, "Running make lint-go-full"),
            (
                self.step_run_make_generate_api_docs,
                "Running make generate-api-docs",
            ),
            (self.step_update_docs, "Updating docs"),
        ]

        total = len(steps)
        make_steps = {
            self.step_run_make_generate,
            self.step_run_make_generate_go_conversions,
            self.step_run_make_manager_only,
            self.step_run_make_lint_go_full,
            self.step_run_make_generate_api_docs,
        }

        for i, (step_func, desc) in enumerate(steps, 1):
            desc_formatted = desc.format(old=self.ctx.old_ver, new=self.ctx.new_ver)
            line = f"[{i}/{total}] {desc_formatted}..."
            print(line, end="", flush=True)

            is_make_step = step_func in make_steps
            if is_make_step:
                self._spinner_line = line
                step_func()
            else:
                exc_holder: list[BaseException] = []

                def run_step() -> None:
                    try:
                        step_func()
                    except BaseException as e:
                        exc_holder.append(e)

                thread = threading.Thread(target=run_step)
                thread.start()
                frame = 0
                while thread.is_alive():
                    spin_char = SPINNER_CHARS[frame % len(SPINNER_CHARS)]
                    sys.stdout.write(
                        f"\r{line} {SPINNER_BRIGHT_WHITE}{spin_char}{SPINNER_RESET}"
                    )
                    sys.stdout.flush()
                    time.sleep(0.1)
                    frame += 1
                thread.join()
                if exc_holder:
                    sys.stdout.write(f"\r{line} {STATUS_FAIL}\n")
                    sys.stdout.flush()
                    raise exc_holder[0]
                sys.stdout.write(f"\r{line} {STATUS_OK}\n")
                sys.stdout.flush()

    # -------------------------------------------------------------------------
    # Step 1: Copy API version directory
    # -------------------------------------------------------------------------

    def step_copy_api_version(self) -> None:
        """Copy api/OLD_VER to api/NEW_VER and replace version references."""
        self.ops.copy_tree(self.ctx.old_api_dir, self.ctx.new_api_dir)
        self.ops.replace_in_tree(
            self.ctx.new_api_dir, self.ctx.old_ver, self.ctx.new_ver
        )

    # -------------------------------------------------------------------------
    # Step 2: Demote old hub to spoke
    # -------------------------------------------------------------------------

    def step_demote_old_hub(self) -> None:
        """Convert Hub()-only conversion files to ConvertTo/ConvertFrom.

        Also handles root types that were introduced in the old hub version
        and therefore have no *_conversion.go file at all.
        """
        for conv_file in self.ctx.old_api_dir.glob("*_conversion.go"):
            content = self.ops.read_file(conv_file)

            # Check if this is a Hub-only file (has Hub() but no ConvertTo)
            if "func (*" not in content or "Hub()" not in content:
                continue
            if "ConvertTo" in content:
                continue

            # Find the corresponding hub file in the new version
            hub_file = self.ctx.new_api_dir / conv_file.name
            if not hub_file.exists():
                continue

            hub_content = self.ops.read_file(hub_file)

            # Extract type names from Hub() declarations
            types = re.findall(r"func \(\*(\w+)\) Hub\(\)", hub_content)
            if not types:
                continue

            # Generate the new conversion file content
            new_content = self._generate_spoke_conversion_file(types)
            self.ops.write_file(conv_file, new_content)

        self._create_conversion_for_new_root_types()

    def _generate_spoke_conversion_file(self, types: list[str]) -> str:
        """Generate spoke conversion file with ConvertTo/ConvertFrom."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        lines = [
            "// © Broadcom. All Rights Reserved.",
            '// The term "Broadcom" refers to Broadcom Inc. and/or its '
            "subsidiaries.",
            "// SPDX-License-Identifier: Apache-2.0",
            "",
            f"package {old_ver}",
            "",
            "import (",
            '\tctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"',
            "",
            f'\tvmopv1 "github.com/vmware-tanzu/vm-operator/api/{new_ver}"',
            ")",
            "",
        ]

        for t in types:
            lines.extend(
                [
                    f"// ConvertTo converts this {t} to the Hub version.",
                    f"func (src *{t}) ConvertTo(dstRaw ctrlconversion.Hub) error {{",
                    f"\tdst := dstRaw.(*vmopv1.{t})",
                    f"\treturn Convert_{old_ver}_{t}_To_{new_ver}_{t}(src, dst, nil)",
                    "}",
                    "",
                    f"// ConvertFrom converts the hub version to this {t}.",
                    f"func (dst *{t}) ConvertFrom(srcRaw ctrlconversion.Hub) error {{",
                    f"\tsrc := srcRaw.(*vmopv1.{t})",
                    f"\treturn Convert_{new_ver}_{t}_To_{old_ver}_{t}(src, dst, nil)",
                    "}",
                    "",
                ]
            )

        return "\n".join(lines)

    def _create_conversion_for_new_root_types(self) -> None:
        """Create conversion files for root types introduced in the old hub
        that have no *_conversion.go file.

        When a type is introduced in the current hub version, it typically
        only gets a types file (no conversion file, since the hub doesn't
        need ConvertTo/ConvertFrom). When a new hub is created, these types
        need:
          1. A spoke conversion file in the old version (ConvertTo/ConvertFrom)
          2. A hub conversion file in the new version (Hub() markers)
        """
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        all_root_types = _discover_root_types(self.ctx.old_api_dir)
        existing_conv_types: set[str] = set()
        for conv_file in self.ctx.old_api_dir.glob("*_conversion.go"):
            content = self.ops.read_file(conv_file)
            for m in re.finditer(r"func \(\w* ?\*(\w+)\)", content):
                existing_conv_types.add(m.group(1))

        missing_types = [t for t in all_root_types if t not in existing_conv_types]
        if not missing_types:
            return

        # Group types by their source file to create appropriately named
        # conversion files.
        type_to_file: dict[str, list[str]] = {}
        root_marker = "// +kubebuilder:object:root=true"
        type_re = re.compile(r"^type\s+(\w+)\s+(?:struct|=)", re.MULTILINE)
        for go_file in sorted(self.ctx.old_api_dir.glob("*_types.go")):
            content = self.ops.read_file(go_file)
            for part in content.split(root_marker)[1:]:
                m = type_re.search(part)
                if m and m.group(1) in missing_types:
                    stem = go_file.stem.replace("_types", "")
                    type_to_file.setdefault(stem, [])
                    type_to_file[stem].append(m.group(1))

        for stem, types in type_to_file.items():
            conv_name = f"{stem}_conversion.go"

            # Include List types alongside their parent types.
            types_with_lists = []
            for t in types:
                types_with_lists.append(t)
                types_with_lists.append(f"{t}List")

            # Create spoke conversion in old version.
            spoke_content = self._generate_spoke_conversion_file(types_with_lists)
            self.ops.write_file(self.ctx.old_api_dir / conv_name, spoke_content)

            # Create hub conversion in new version.
            hub_content = self._generate_hub_conversion_file(
                new_ver, types_with_lists
            )
            self.ops.write_file(self.ctx.new_api_dir / conv_name, hub_content)

    @staticmethod
    def _generate_hub_conversion_file(package_name: str, types: list[str]) -> str:
        """Generate a hub conversion file with Hub() markers."""
        lines = [
            "// © Broadcom. All Rights Reserved.",
            '// The term "Broadcom" refers to Broadcom Inc. and/or its '
            "subsidiaries.",
            "// SPDX-License-Identifier: Apache-2.0",
            "",
            f"package {package_name}",
            "",
        ]

        for t in types:
            lines.extend(
                [
                    f"// Hub marks {t} as a conversion hub.",
                    f"func (*{t}) Hub() {{}}",
                    "",
                ]
            )

        return "\n".join(lines)

    # -------------------------------------------------------------------------
    # Step 3: Remove storage version marker
    # -------------------------------------------------------------------------

    def step_remove_storage_version(self) -> None:
        """Remove +kubebuilder:storageversion from old API version."""
        for go_file in self.ctx.old_api_dir.rglob("*.go"):
            self.ops.remove_lines_matching(
                go_file, r"^\s*//\s*\+kubebuilder:storageversion\s*$"
            )

    # -------------------------------------------------------------------------
    # Step 4: Add localSchemeBuilder
    # -------------------------------------------------------------------------

    def step_add_local_scheme_builder(self) -> None:
        """Add localSchemeBuilder to groupversion_info.go for conversion-gen."""
        gv_info = self.ctx.old_api_dir / "groupversion_info.go"
        if not gv_info.exists():
            return

        content = self.ops.read_file(gv_info)
        if "localSchemeBuilder" in content:
            return

        def add_local_scheme_builder(text: str) -> str:
            pattern = r"(objectTypes = \[\].*runtime\.Object\{\})"
            replacement = (
                r"\1\n\n"
                "\t// localSchemeBuilder is used for type conversions.\n"
                "\tlocalSchemeBuilder = &schemeBuilder"
            )
            return re.sub(pattern, replacement, text)

        self.ops.transform_file(gv_info, add_local_scheme_builder)

    # -------------------------------------------------------------------------
    # Step 5: Update api/apis.go
    # -------------------------------------------------------------------------

    def step_update_apis_go(self) -> None:
        """Update api/apis.go with new imports and AddToScheme calls."""
        apis_go = self.ctx.api_dir / "apis.go"
        if not apis_go.exists():
            return

        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        alias = self.ctx.old_ver_alias

        content = self.ops.read_file(apis_go)

        # Replace vmopv1 import with aliased import for old version
        old_import = f'vmopv1 "github.com/vmware-tanzu/vm-operator/api/{old_ver}"'
        new_import = f'{alias} "github.com/vmware-tanzu/vm-operator/api/{old_ver}"'
        content = content.replace(old_import, new_import)

        # Add vmopv1 import for new version after the aliased import
        alias_import_line = (
            f'\t{alias} "github.com/vmware-tanzu/vm-operator/api/{old_ver}"'
        )
        vmopv1_import_line = (
            f'\tvmopv1 "github.com/vmware-tanzu/vm-operator/api/{new_ver}"'
        )
        if alias_import_line in content and vmopv1_import_line not in content:
            content = content.replace(
                alias_import_line, f"{alias_import_line}\n{vmopv1_import_line}"
            )

        # Add AddToScheme call for old version before vmopv1.AddToScheme
        add_to_scheme_pattern = r"(\treturn vmopv1\.AddToScheme\(s\))"
        add_to_scheme_replacement = (
            f"\tif err := {alias}.AddToScheme(s); err != nil {{\n"
            f"\t\treturn err\n"
            f"\t}}\n"
            r"\1"
        )
        # Only add if not already present
        if f"{alias}.AddToScheme" not in content:
            content = re.sub(add_to_scheme_pattern, add_to_scheme_replacement, content)

        self.ops.write_file(apis_go, content)

    # -------------------------------------------------------------------------
    # Step 6: Update doc.go conversion-gen targets
    # -------------------------------------------------------------------------

    def step_update_doc_conversion_gen(self) -> None:
        """Update conversion-gen targets in doc.go files."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        # Update all doc.go files that reference the old version
        for doc_file in self.ctx.api_dir.glob("*/doc.go"):
            content = self.ops.read_file(doc_file)
            if (
                f"conversion-gen=.*api/{old_ver}" in content
                or f"api/{old_ver}" in content
            ):
                self.ops.replace_in_file(doc_file, f"api/{old_ver}", f"api/{new_ver}")

        # Ensure old version's doc.go has conversion-gen pointing to new version
        old_doc = self.ctx.old_api_dir / "doc.go"
        if old_doc.exists():
            content = self.ops.read_file(old_doc)
            if "+k8s:conversion-gen" not in content:
                # Insert conversion-gen before "// Package" (blank line so
                # package comment is its own block for revive).
                def add_conversion_gen(text: str) -> str:
                    conv_gen = (
                        f"// +k8s:conversion-gen=github.com/vmware-tanzu/"
                        f"vm-operator/api/{new_ver}\n\n\\1"
                    )
                    return re.sub(
                        r"(// Package )",
                        conv_gen,
                        text,
                        count=1,
                    )

                self.ops.transform_file(old_doc, add_conversion_gen)
            else:
                # Ensure blank line between conversion-gen and "// Package ..."
                # so revive package-comments is satisfied.
                new_content = re.sub(
                    r"(// \+k8s:conversion-gen=[^\n]+)\n(// Package )",
                    r"\1\n\n\2",
                    content,
                    count=1,
                )
                if new_content != content:
                    self.ops.write_file(old_doc, new_content)

    # -------------------------------------------------------------------------
    # Step 7: Create conversion subdirs for old version
    # -------------------------------------------------------------------------

    def step_create_conversion_subdirs(self) -> None:
        """Create conversion subdirs for sysprep and common subpackages."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        prev_ver = self.ctx.prev_ver

        if not prev_ver:
            return

        for sub in ["sysprep", "common"]:
            sub_dir = self.ctx.old_api_dir / sub
            conv_dir = sub_dir / "conversion"

            if not sub_dir.exists() or conv_dir.exists():
                continue

            # Find a previous version with conversion dirs
            source_ver = self._find_version_with_conversion(sub, old_ver)
            if not source_ver:
                continue

            source_conv_dir = self.ctx.api_dir / source_ver / sub / "conversion"
            self.ops.mkdir(conv_dir)

            prev_alias = self.ctx.version_alias(source_ver)
            old_alias = self.ctx.version_alias(old_ver)

            # Copy spoke conversion dir (spoke -> hub)
            spoke_src = source_conv_dir / source_ver
            if spoke_src.exists():
                spoke_dst = conv_dir / old_ver
                self.ops.copy_tree(spoke_src, spoke_dst)
                self._transform_conversion_dir(
                    spoke_dst,
                    source_ver,
                    old_ver,
                    new_ver,
                    prev_alias,
                    old_alias,
                    is_spoke=True,
                )

            # Copy hub conversion dir (hub -> spoke)
            hub_src = source_conv_dir / old_ver
            if hub_src.exists():
                hub_dst = conv_dir / new_ver
                self.ops.copy_tree(hub_src, hub_dst)
                self._transform_conversion_dir(
                    hub_dst,
                    source_ver,
                    old_ver,
                    new_ver,
                    prev_alias,
                    old_alias,
                    is_spoke=False,
                )

    def _find_version_with_conversion(self, subpkg: str, before_ver: str) -> str | None:
        """Find version with conversion dirs for subpkg, before given ver."""
        kv_before = KubeVersion.parse(before_ver)
        if not kv_before:
            return None

        for v in reversed(self.ctx.all_versions):
            if v == before_ver or v == self.ctx.new_ver:
                continue
            kv = KubeVersion.parse(v)
            if kv and kv < kv_before:
                conv_dir = self.ctx.api_dir / v / subpkg / "conversion"
                if conv_dir.exists():
                    return v
        return None

    def _transform_conversion_dir(
        self,
        conv_dir: Path,
        source_ver: str,
        old_ver: str,
        new_ver: str,
        prev_alias: str,
        old_alias: str,
        is_spoke: bool,
    ) -> None:
        """Transform a copied conversion directory."""
        # Replace version strings
        self.ops.replace_in_tree(conv_dir, source_ver, old_ver)
        self.ops.replace_in_tree(conv_dir, old_ver, new_ver)
        self.ops.replace_in_tree(conv_dir, prev_alias, old_alias)

        # Fix imports and package declarations
        for go_file in conv_dir.rglob("*.go"):
            self._fix_conversion_imports(
                go_file, old_alias, old_ver, new_ver, fix_package=is_spoke
            )

    def _fix_conversion_imports(
        self,
        path: Path,
        alias: str,
        old_ver: str,
        new_ver: str,
        fix_package: bool = False,
    ) -> None:
        """Fix spoke alias imports and package declarations in conversion files.

        The spoke alias (e.g. vmopv1a5common) must import api/old_ver, not
        api/new_ver. In conversion/OLD_VER (spoke dir), package must be
        old_ver; in conversion/NEW_VER (hub dir), package must remain new_ver.
        """
        content = self.ops.read_file(path)

        # Fix import: spoke alias must import api/old_ver/..., not new_ver/...
        # Alias in context is base (vmopv1a5); imports use base+subpkg
        # (vmopv1a5common, vmopv1a5sysprep).
        content = re.sub(
            rf'({re.escape(alias)}[a-zA-Z0-9]*\s+"[^"]*/)({re.escape(new_ver)})([^"]*")',
            rf"\g<1>{old_ver}\3",
            content,
        )

        # Fix package declaration: should be old_ver in spoke dirs
        content = re.sub(
            rf"^package {new_ver}$",
            f"package {old_ver}",
            content,
            flags=re.MULTILINE,
        )

        self.ops.write_file(path, content)

    # -------------------------------------------------------------------------
    # Step 8: Update Makefile
    # -------------------------------------------------------------------------

    def step_update_makefile(self) -> None:
        """Update Makefile: conversion-gen package list and EXTRA_PEER_DIRS."""
        makefile = self.ctx.root / "Makefile"
        if not makefile.exists():
            return

        content = self.ops.read_file(makefile)

        # Update conversion-gen package list
        all_versions = sort_versions(self.ctx.all_versions + [self.ctx.new_ver])
        pkg_list = " ".join(f"./{v}" for v in all_versions)

        def update_conversion_gen_packages(text: str) -> str:
            # Match lines starting with tabs and version directories
            return re.sub(r"(\t\t)\./v[0-9][^\n]*", rf"\1{pkg_list}", text, count=1)

        content = update_conversion_gen_packages(content)

        # Add EXTRA_PEER_DIRS for new conversion directories
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        prev_ver = self.ctx.prev_ver

        if prev_ver and (self.ctx.old_api_dir / "sysprep" / "conversion").exists():
            peer_dirs_marker = f"{prev_ver}/common/conversion/{old_ver}"
            if (
                peer_dirs_marker in content
                and f"{old_ver}/sysprep/conversion/{old_ver}" not in content
            ):
                new_peer_dirs = "\n".join(
                    [
                        f"EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./{old_ver}/sysprep/conversion/{old_ver}",
                        f"EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./{old_ver}/sysprep/conversion/{new_ver}",
                        f"EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./{old_ver}/common/conversion/{old_ver}",
                        f"EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./{old_ver}/common/conversion/{new_ver}",
                    ]
                )
                content = content.replace(
                    f"EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./{prev_ver}/common/conversion/{old_ver}",
                    f"EXTRA_PEER_DIRS := $(EXTRA_PEER_DIRS),./{prev_ver}/common/conversion/{old_ver}\n{new_peer_dirs}",
                )

        self.ops.write_file(makefile, content)

    # -------------------------------------------------------------------------
    # Step 9: Create webhooks/conversion for new version
    # -------------------------------------------------------------------------

    def step_create_webhooks_conversion(self) -> None:
        """Create webhooks/conversion/NEW_VER with webhooks.go registering all root types from previous hub."""
        webhooks_dir = self.ctx.root / "webhooks" / "conversion"
        old_api_dir = self.ctx.old_api_dir
        new_webhook_dir = webhooks_dir / self.ctx.new_ver
        old_webhook_dir = webhooks_dir / self.ctx.old_ver
        webhooks_go = webhooks_dir / "webhooks.go"

        if not old_api_dir.exists():
            return

        root_types = _discover_root_types(old_api_dir)
        if not root_types:
            return

        # Create webhooks/conversion/NEW_VER/webhooks.go for the new hub.
        new_webhook_dir.mkdir(parents=True, exist_ok=True)
        api_import_path = f"github.com/vmware-tanzu/vm-operator/api/{self.ctx.new_ver}"
        package_name = self.ctx.new_ver
        content = _generate_conversion_webhooks_go(
            package_name, api_import_path, root_types
        )
        self.ops.write_file(new_webhook_dir / "webhooks.go", content)

        # Regenerate webhooks/conversion/OLD_VER/webhooks.go so it includes
        # all root types (including types introduced in the old hub version
        # that were not present in the previous hub's webhook file).
        old_webhook_dir.mkdir(parents=True, exist_ok=True)
        old_alias = self.ctx.version_alias(self.ctx.old_ver)
        old_api_import = (
            f"github.com/vmware-tanzu/vm-operator/api/{self.ctx.old_ver}"
        )
        old_content = _generate_conversion_webhooks_go(
            self.ctx.old_ver, old_api_import, root_types,
            import_alias=old_alias,
        )
        self.ops.write_file(old_webhook_dir / "webhooks.go", old_content)

        # Update parent webhooks/conversion/webhooks.go with new imports and AddToManager call
        if webhooks_go.exists():
            self._update_webhooks_go(webhooks_go)

    def _update_webhooks_go(self, webhooks_go: Path) -> None:
        """Update webhooks/conversion/webhooks.go with new imports and calls."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        alias = self.ctx.old_ver_alias

        content = self.ops.read_file(webhooks_go)

        # Replace vmopv1 import with aliased import
        old_import = f'vmopv1 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/{old_ver}"'
        new_import = f'{alias} "github.com/vmware-tanzu/vm-operator/webhooks/conversion/{old_ver}"'
        content = content.replace(old_import, new_import)

        # Add vmopv1 import for new version
        alias_import = f'\t{alias} "github.com/vmware-tanzu/vm-operator/webhooks/conversion/{old_ver}"'
        vmopv1_import = f'\tvmopv1 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/{new_ver}"'
        if alias_import in content and vmopv1_import not in content:
            content = content.replace(alias_import, f"{alias_import}\n{vmopv1_import}")

        # Add AddToManager call for old version
        add_to_manager = "if err := vmopv1.AddToManager(ctx, mgr); err != nil {"
        if f"{alias}.AddToManager" not in content:
            new_call = (
                f"if err := {alias}.AddToManager(ctx, mgr); err != nil {{\n"
                f"\t\treturn err\n"
                f"\t}}\n\t"
            )
            content = content.replace(add_to_manager, new_call + add_to_manager)

        self.ops.write_file(webhooks_go, content)

    # -------------------------------------------------------------------------
    # Step 10: Update older API versions to reference new hub
    # -------------------------------------------------------------------------

    def step_update_older_api_versions(self) -> None:
        """Update older API versions to use new hub version."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        for ver in self.ctx.older_versions():
            ver_dir = self.ctx.api_dir / ver
            if not ver_dir.exists():
                continue

            # Replace old_ver with new_ver in all files
            self.ops.replace_in_tree(ver_dir, old_ver, new_ver)

            # Rename conversion subdirs
            for sub in ["sysprep", "common"]:
                old_conv_dir = ver_dir / sub / "conversion" / old_ver
                new_conv_dir = ver_dir / sub / "conversion" / new_ver
                if old_conv_dir.exists():
                    self.ops.move(old_conv_dir, new_conv_dir)

        # Update Makefile EXTRA_PEER_DIRS
        makefile = self.ctx.root / "Makefile"
        if makefile.exists():
            for ver in self.ctx.older_versions():
                for sub in ["sysprep", "common"]:
                    self.ops.replace_in_file(
                        makefile,
                        f"./{ver}/{sub}/conversion/{old_ver}",
                        f"./{ver}/{sub}/conversion/{new_ver}",
                    )

    # -------------------------------------------------------------------------
    # Step 11: Update webhook annotations and paths
    # -------------------------------------------------------------------------

    def step_update_webhook_annotations(self) -> None:
        """Update webhook annotations and test constants."""
        dirs_to_update = [
            "webhooks/virtualmachine",
            "webhooks/virtualmachineclass",
            "webhooks/virtualmachinepublishrequest",
            "webhooks/virtualmachinereplicaset",
            "webhooks/virtualmachineservice",
            "webhooks/virtualmachinesnapshot",
            "webhooks/virtualmachinesetresourcepolicy",
            "webhooks/virtualmachinegroup",
            "webhooks/virtualmachinegrouppublishrequest",
            "webhooks/virtualmachinewebconsolerequest",
            "controllers",
        ]

        for dir_path in dirs_to_update:
            full_path = self.ctx.root / dir_path
            if full_path.exists():
                self.ops.replace_in_tree(full_path, self.ctx.old_ver, self.ctx.new_ver)

    # -------------------------------------------------------------------------
    # Step 12: Update imports throughout codebase
    # -------------------------------------------------------------------------

    def step_update_imports(self) -> None:
        """Update api/OLD_VER imports to api/NEW_VER throughout the codebase."""
        old_import = f"github.com/vmware-tanzu/vm-operator/api/{self.ctx.old_ver}"
        new_import = f"github.com/vmware-tanzu/vm-operator/api/{self.ctx.new_ver}"

        exclude_paths = {
            self.ctx.api_dir / "apis.go",
        }
        exclude_prefixes = [
            self.ctx.old_api_dir,
            self.ctx.new_api_dir,
            self.ctx.root / "vendor",
            self.ctx.root / "webhooks" / "conversion" / self.ctx.old_ver,
        ]

        for go_file in self.ctx.root.rglob("*.go"):
            if go_file in exclude_paths:
                continue
            if any(go_file.is_relative_to(prefix) for prefix in exclude_prefixes):
                continue

            self.ops.replace_in_file(go_file, old_import, new_import)

    # -------------------------------------------------------------------------
    # Step 13: Update unit and integration tests
    # -------------------------------------------------------------------------

    def step_update_vmgroup_test_panic_message(self) -> None:
        """Update expected type in PanicWith string to match new hub (e.g. v1alpha6 -> v1alpha7)."""
        old_num = self._version_alpha_num(self.ctx.old_ver)
        new_num = self._version_alpha_num(self.ctx.new_ver)
        if not old_num or not new_num:
            return

        path = self.ctx.root / "pkg/util/vmopv1/vmgroup_test.go"
        if not path.exists():
            return

        content = self.ops.read_file(path)
        old_str = (
            f'"Expected PolicyEvaluation, but got *v1alpha{old_num}.VirtualMachine"'
        )
        new_str = (
            f'"Expected PolicyEvaluation, but got *v1alpha{new_num}.VirtualMachine"'
        )
        if old_str not in content:
            return
        content = content.replace(old_str, new_str, 1)
        self.ops.write_file(path, content)

    # -------------------------------------------------------------------------
    # Step 14: Update .golangci.yml importas (demote old hub, add new hub)
    # -------------------------------------------------------------------------

    def step_update_golangci_importas(self) -> None:
        """Update importas: OLD_VER -> versioned alias, NEW_VER -> vmopv1."""
        golangci = self.ctx.root / ".golangci.yml"
        if not golangci.exists():
            return

        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        old_alias = self.ctx.version_alias(old_ver)
        content = self.ops.read_file(golangci)

        base = "github.com/vmware-tanzu/vm-operator/api"
        # Block mapping vmopv1 (and subpackages) to old_ver; replace with
        # versioned alias block + new hub block.
        old_block = (
            f"        - alias: vmopv1\n"
            f"          pkg: {base}/{old_ver}\n"
            f"        - alias: vmopv1cloudinit\n"
            f"          pkg: {base}/{old_ver}/cloudinit\n"
            f"        - alias: vmopv1common\n"
            f"          pkg: {base}/{old_ver}/common\n"
            f"        - alias: vmopv1sysprep\n"
            f"          pkg: {base}/{old_ver}/sysprep"
        )
        new_block = (
            f"        - alias: {old_alias}\n"
            f"          pkg: {base}/{old_ver}\n"
            f"        - alias: {old_alias}cloudinit\n"
            f"          pkg: {base}/{old_ver}/cloudinit\n"
            f"        - alias: {old_alias}common\n"
            f"          pkg: {base}/{old_ver}/common\n"
            f"        - alias: {old_alias}sysprep\n"
            f"          pkg: {base}/{old_ver}/sysprep\n"
            f"\n"
            f"        - alias: vmopv1\n"
            f"          pkg: {base}/{new_ver}\n"
            f"        - alias: vmopv1cloudinit\n"
            f"          pkg: {base}/{new_ver}/cloudinit\n"
            f"        - alias: vmopv1common\n"
            f"          pkg: {base}/{new_ver}/common\n"
            f"        - alias: vmopv1sysprep\n"
            f"          pkg: {base}/{new_ver}/sysprep"
        )
        if old_block in content:
            self.ops.write_file(golangci, content.replace(old_block, new_block, 1))
            return

        # Fallback: no cloudinit (e.g. some versions)
        old_block_simple = (
            f"        - alias: vmopv1\n"
            f"          pkg: {base}/{old_ver}\n"
            f"        - alias: vmopv1common\n"
            f"          pkg: {base}/{old_ver}/common\n"
            f"        - alias: vmopv1sysprep\n"
            f"          pkg: {base}/{old_ver}/sysprep"
        )
        new_block_simple = (
            f"        - alias: {old_alias}\n"
            f"          pkg: {base}/{old_ver}\n"
            f"        - alias: {old_alias}common\n"
            f"          pkg: {base}/{old_ver}/common\n"
            f"        - alias: {old_alias}sysprep\n"
            f"          pkg: {base}/{old_ver}/sysprep\n"
            f"\n"
            f"        - alias: vmopv1\n"
            f"          pkg: {base}/{new_ver}\n"
            f"        - alias: vmopv1common\n"
            f"          pkg: {base}/{new_ver}/common\n"
            f"        - alias: vmopv1sysprep\n"
            f"          pkg: {base}/{new_ver}/sysprep"
        )
        if old_block_simple in content:
            self.ops.write_file(
                golangci, content.replace(old_block_simple, new_block_simple, 1)
            )

    # -------------------------------------------------------------------------
    # Step 15: Fix api/OLD_VER import aliases to match .golangci importas
    # -------------------------------------------------------------------------

    def step_fix_old_ver_import_aliases(self) -> None:
        """Update api/OLD_VER so same-version subpackage imports use versioned
        alias (vmopv1a5common, etc.).
        """
        old_ver = self.ctx.old_ver
        old_alias = self.ctx.version_alias(old_ver)
        base = "github.com/vmware-tanzu/vm-operator/api"
        old_api = self.ctx.old_api_dir

        # Replace import lines (all files under api/OLD_VER, incl. conversion)
        import_replacements = [
            (
                f'vmopv1common "{base}/{old_ver}/common"',
                f'{old_alias}common "{base}/{old_ver}/common"',
            ),
            (
                f'vmopv1cloudinit "{base}/{old_ver}/cloudinit"',
                f'{old_alias}cloudinit "{base}/{old_ver}/cloudinit"',
            ),
            (
                f'vmopv1sysprep "{base}/{old_ver}/sysprep"',
                f'{old_alias}sysprep "{base}/{old_ver}/sysprep"',
            ),
        ]
        for old_import, new_import in import_replacements:
            self.ops.replace_in_tree(old_api, old_import, new_import)

        # Replace type usages (exclude conversion dirs; vmopv1common = hub)
        exclude_conversion = lambda p: "conversion" not in p.parts
        usage_replacements = [
            ("vmopv1common.", f"{old_alias}common."),
            ("vmopv1cloudinit.", f"{old_alias}cloudinit."),
            ("vmopv1sysprep.", f"{old_alias}sysprep."),
        ]
        for old_usage, new_usage in usage_replacements:
            self.ops.replace_in_tree(
                old_api, old_usage, new_usage, file_filter=exclude_conversion
            )

    # -------------------------------------------------------------------------
    # Step 16: Add new version template constants (pkg/providers/vsphere/constants)
    # -------------------------------------------------------------------------

    def _version_alpha_num(self, version: str) -> str | None:
        """Return the numeric part of v1alphaN or v1betaN (e.g. v1alpha5 -> '5')."""
        m = re.search(r"v1alpha(\d+)", version)
        if m:
            return m.group(1)
        m = re.search(r"v1beta(\d+)", version)
        if m:
            return m.group(1)
        return None

    def step_update_template_constants(self) -> None:
        """Add new hub version template function constants to constants/constants.go."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        new_ver_go = self._version_go_form(new_ver)
        new_num = self._version_alpha_num(new_ver)
        if not new_num:
            return

        constants_go = self.ctx.root / "pkg/providers/vsphere/constants/constants.go"
        if not constants_go.exists():
            return

        content = self.ops.read_file(constants_go)
        # Already has new version constants
        if f"V1alpha{new_num}FirstIP" in content or f"{new_ver_go}FirstIP" in content:
            return

        old_ver_go = self._version_go_form(old_ver)
        # Insert new version block after old version's FormatNameservers line
        marker = (
            f"\t// {old_ver_go}FormatNameservers is an alias for versioned "
            f"templating function {old_ver_go}_FormatNameservers.\n"
            f'\t{old_ver_go}FormatNameservers = "{old_ver_go}_FormatNameservers"\n)'
        )
        new_block = (
            f"\t// {old_ver_go}FormatNameservers is an alias for versioned "
            f"templating function {old_ver_go}_FormatNameservers.\n"
            f'\t{old_ver_go}FormatNameservers = "{old_ver_go}_FormatNameservers"\n\n'
            f"\t// {new_ver_go}FirstIP is an alias for versioned templating "
            f"function {new_ver_go}_FirstIP.\n"
            f'\t{new_ver_go}FirstIP = "{new_ver_go}_FirstIP"\n'
            f"\t// {new_ver_go}FirstNicMacAddr is an alias for versioned templating "
            f"function {new_ver_go}_FirstNicMacAddr.\n"
            f'\t{new_ver_go}FirstNicMacAddr = "{new_ver_go}_FirstNicMacAddr"\n'
            f"\t// {new_ver_go}FirstIPFromNIC is an alias for versioned templating "
            f"function {new_ver_go}_FirstIPFromNIC.\n"
            f'\t{new_ver_go}FirstIPFromNIC = "{new_ver_go}_FirstIPFromNIC"\n'
            f"\t// {new_ver_go}IPsFromNIC is an alias for versioned templating "
            f"function {new_ver_go}_IPsFromNIC.\n"
            f'\t{new_ver_go}IPsFromNIC = "{new_ver_go}_IPsFromNIC"\n'
            f"\t// {new_ver_go}FormatIP is an alias for versioned templating "
            f"function {new_ver_go}_FormatIP.\n"
            f'\t{new_ver_go}FormatIP = "{new_ver_go}_FormatIP"\n'
            f"\t// {new_ver_go}IP is an alias for versioned templating "
            f"function {new_ver_go}_IP.\n"
            f'\t{new_ver_go}IP = "{new_ver_go}_IP"\n'
            f"\t// {new_ver_go}SubnetMask is an alias for versioned templating "
            f"function  {new_ver_go}_SubnetMask.\n"
            f'\t{new_ver_go}SubnetMask = "{new_ver_go}_SubnetMask"\n'
            f"\t// {new_ver_go}FormatNameservers is an alias for versioned "
            f"templating function {new_ver_go}_FormatNameservers.\n"
            f'\t{new_ver_go}FormatNameservers = "{new_ver_go}_FormatNameservers"\n)'
        )
        if marker not in content:
            return
        self.ops.write_file(constants_go, content.replace(marker, new_block, 1))

    # -------------------------------------------------------------------------
    # Step 17: Demote old hub and add new hub in bootstrap_templatedata.go
    # -------------------------------------------------------------------------

    def step_update_bootstrap_templatedata(self) -> None:
        """Demote previous hub to spoke and introduce new hub in bootstrap_templatedata.go."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        old_alias = self.ctx.version_alias(old_ver)
        old_ver_go = self._version_go_form(old_ver)
        new_ver_go = self._version_go_form(new_ver)
        old_num = self._version_alpha_num(old_ver)
        new_num = self._version_alpha_num(new_ver)
        if not old_num or not new_num:
            return

        path = (
            self.ctx.root
            / "pkg/providers/vsphere/vmlifecycle/bootstrap_templatedata.go"
        )
        if not path.exists():
            return

        content = self.ops.read_file(path)
        base_import = "github.com/vmware-tanzu/vm-operator/api"

        # Already updated (has old version import and new hub block)
        if f'{old_alias} "{base_import}/{old_ver}"' in content and (
            f"networkDevicesStatusV1A{new_num}" in content
            or f"toTemplateNetworkStatusV1A{new_num}" in content
        ):
            return

        # 1) Add old version import before vmopv1 (vmopv1 currently points to new_ver after step_update_imports)
        vmopv1_new_line = f'\tvmopv1 "{base_import}/{new_ver}"'
        old_import_line = f'\t{old_alias} "{base_import}/{old_ver}"'
        if old_import_line not in content:
            content = content.replace(
                vmopv1_new_line,
                f"{old_import_line}\n{vmopv1_new_line}",
                1,
            )

        # 2) Demote old hub: networkStatusV1A{old_num} use old_alias, add V1A{new_num} block and v1a{old_num}VM
        old_status_type = f"networkStatusV1A{old_num} := vmopv1.NetworkStatus"
        new_status_old = f"networkStatusV1A{old_num} := {old_alias}.NetworkStatus"
        content = content.replace(old_status_type, new_status_old, 1)

        # Insert V1A{new_num} network block and v1a{old_num}VM after old status block, before "// Use separate deep"
        old_block_end = (
            f"\t\tNameservers: bsArgs.DNSServers,\n"
            f"\t}}\n\n"
            f"\t// Use separate deep copies"
        )
        new_block_mid = (
            f"\t\tNameservers: bsArgs.DNSServers,\n"
            f"\t}}\n\n"
            f"\tnetworkDevicesStatusV1A{new_num} := toTemplateNetworkStatusV1A{new_num}(bsArgs)\n"
            f"\tnetworkStatusV1A{new_num} := vmopv1.NetworkStatus{{\n"
            f"\t\tDevices:     networkDevicesStatusV1A{new_num},\n"
            f"\t\tNameservers: bsArgs.DNSServers,\n"
            f"\t}}\n\n"
            f"\t// Use separate deep copies"
        )
        content = content.replace(old_block_end, new_block_mid, 1)

        # 3) Add v1a{old_num}VM conversion after v1a{old_num-1}VM
        prev_num = str(int(old_num) - 1) if old_num.isdigit() else None
        if prev_num:
            v1a_prev_convert = (
                f"v1a{prev_num}VM := &vmopv1a{prev_num}.VirtualMachine{{}}\n"
                f"\t_ = v1a{prev_num}VM.ConvertFrom(vmCtx.VM.DeepCopy())\n\n"
                f"\ttemplateData :="
            )
            v1a_prev_convert_new = (
                f"v1a{prev_num}VM := &vmopv1a{prev_num}.VirtualMachine{{}}\n"
                f"\t_ = v1a{prev_num}VM.ConvertFrom(vmCtx.VM.DeepCopy())\n\n"
                f"\tv1a{old_num}VM := &{old_alias}.VirtualMachine{{}}\n"
                f"\t_ = v1a{old_num}VM.ConvertFrom(vmCtx.VM.DeepCopy())\n\n"
                f"\ttemplateData :="
            )
            if v1a_prev_convert in content and f"v1a{old_num}VM :=" not in content:
                content = content.replace(v1a_prev_convert, v1a_prev_convert_new, 1)

        # 4) templateData struct: old field use old_alias, add new field
        # Struct closing is "}{\n" on one line (not "}\n" then "{\n"). Try 1 or 2 tabs.
        struct_old_1tab = f"{old_ver_go} vmopv1.VirtualMachineTemplate\n\t}}{{\n"
        struct_new_1tab = (
            f"{old_ver_go} {old_alias}.VirtualMachineTemplate\n"
            f"\t\t{new_ver_go} vmopv1.VirtualMachineTemplate\n\t}}{{\n"
        )
        struct_old_2tab = f"{old_ver_go} vmopv1.VirtualMachineTemplate\n\t\t}}{{\n"
        struct_new_2tab = (
            f"{old_ver_go} {old_alias}.VirtualMachineTemplate\n"
            f"\t\t{new_ver_go} vmopv1.VirtualMachineTemplate\n\t\t}}{{\n"
        )
        if struct_old_1tab in content:
            content = content.replace(struct_old_1tab, struct_new_1tab, 1)
        elif struct_old_2tab in content:
            content = content.replace(struct_old_2tab, struct_new_2tab, 1)

        # 5) templateData literal: old use v1a{old_num}VM, add new entry
        # File uses three tabs before Net/VM (inside the struct literal)
        old_literal = (
            f"{old_ver_go}: vmopv1.VirtualMachineTemplate{{\n"
            f"\t\t\tNet: networkStatusV1A{old_num},\n"
            f"\t\t\tVM:  vmCtx.VM,\n"
            "\t\t},\n"
            "\t}"
        )
        new_literal = (
            f"{old_ver_go}: {old_alias}.VirtualMachineTemplate{{\n"
            f"\t\t\tNet: networkStatusV1A{old_num},\n"
            f"\t\t\tVM:  v1a{old_num}VM,\n"
            "\t\t},\n"
            f"\t\t{new_ver_go}: vmopv1.VirtualMachineTemplate{{\n"
            f"\t\t\tNet: networkStatusV1A{new_num},\n"
            f"\t\t\tVM:  vmCtx.VM,\n"
            "\t\t},\n"
            "\t}"
        )
        content = content.replace(old_literal, new_literal, 1)

        # 6) Add v1a{new_num}FuncMap and merge into funcMap
        content = content.replace(
            f"v1a{old_num}FuncMap := v1a{old_num}TemplateFunctions("
            f"networkStatusV1A{old_num}, networkDevicesStatusV1A{old_num})\n\n"
            f"\t// Include all but",
            f"v1a{old_num}FuncMap := v1a{old_num}TemplateFunctions("
            f"networkStatusV1A{old_num}, networkDevicesStatusV1A{old_num})\n"
            f"\tv1a{new_num}FuncMap := v1a{new_num}TemplateFunctions("
            f"networkStatusV1A{new_num}, networkDevicesStatusV1A{new_num})\n\n"
            f"\t// Include all but",
            1,
        )
        content = content.replace(
            f"for k, v := range v1a{old_num}FuncMap {{\n" f"\t\tfuncMap[k] = v\n\t}}\n",
            f"for k, v := range v1a{old_num}FuncMap {{\n"
            f"\t\tfuncMap[k] = v\n\t}}\n"
            f"\tfor k, v := range v1a{new_num}FuncMap {{\n"
            f"\t\tfuncMap[k] = v\n\t}}\n",
            1,
        )

        # 7) toTemplateNetworkStatusV1A{old_num}: return and body use old_alias
        content = content.replace(
            f"func toTemplateNetworkStatusV1A{old_num}(bsArgs *BootstrapArgs) "
            f"[]vmopv1.NetworkDeviceStatus",
            f"func toTemplateNetworkStatusV1A{old_num}(bsArgs *BootstrapArgs) "
            f"[]{old_alias}.NetworkDeviceStatus",
            1,
        )
        content = content.replace(
            f"make([]vmopv1.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))",
            f"make([]{old_alias}.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))",
            1,
        )
        status_old = "status := vmopv1.NetworkDeviceStatus{\n\t\t\tMacAddress: macAddr,"
        status_new = (
            f"status := {old_alias}.NetworkDeviceStatus{{\n\t\t\tMacAddress: macAddr,"
        )
        content = content.replace(status_old, status_new, 1)

        # 8) Insert toTemplateNetworkStatusV1A{new_num} after toTemplateNetworkStatusV1A{old_num}
        # In the file, the next function after toTemplateNetworkStatusV1A{old_num} is
        # v1a3TemplateFunctions (order is V1A1,v1a1, V1A2,v1a2, V1A3,V1A4,V1A5, v1a3,v1a4,v1a5).
        insert_after = (
            "return networkDevicesStatus\n"
            "}\n\n"
            "// This is basically identical to v1a2TemplateFunctions.\n"
            "func v1a3TemplateFunctions("
        )
        new_func_template = (
            "return networkDevicesStatus\n"
            "}\n\n"
            f"func toTemplateNetworkStatusV1A{new_num}(bsArgs *BootstrapArgs) "
            f"[]vmopv1.NetworkDeviceStatus {{\n"
            f"\tnetworkDevicesStatus := make([]vmopv1.NetworkDeviceStatus, 0, "
            "len(bsArgs.NetworkResults.Results))\n\n"
            "\tfor _, result := range bsArgs.NetworkResults.Results {\n"
            '\t\tmacAddr := strings.ReplaceAll(result.MacAddress, ":", "-")\n\n'
            "\t\tstatus := vmopv1.NetworkDeviceStatus{\n"
            "\t\t\tMacAddress: macAddr,\n"
            "\t\t}\n\n"
            "\t\tfor _, ipConfig := range result.IPConfigs {\n"
            "\t\t\tif ipConfig.IsIPv4 {\n"
            '\t\t\t\tif status.Gateway4 == "" {\n'
            "\t\t\t\t\tstatus.Gateway4 = ipConfig.Gateway\n"
            "\t\t\t\t}\n\n"
            "\t\t\t\tstatus.IPAddresses = append(status.IPAddresses, "
            "ipConfig.IPCIDR)\n"
            "\t\t\t}\n"
            "\t\t}\n\n"
            "\t\tnetworkDevicesStatus = append(networkDevicesStatus, status)\n"
            "\t}\n\n"
            "\treturn networkDevicesStatus\n"
            "}\n\n"
            "// This is basically identical to v1a2TemplateFunctions.\n"
            "func v1a3TemplateFunctions("
        )
        if f"func toTemplateNetworkStatusV1A{new_num}(" not in content:
            content = content.replace(insert_after, new_func_template, 1)

        # 9) v1a{old_num}TemplateFunctions: param types to old_alias
        content = content.replace(
            f"func v1a{old_num}TemplateFunctions(\n"
            f"\tnetworkStatusV1A{old_num} vmopv1.NetworkStatus,\n"
            f"\tnetworkDevicesStatusV1A{old_num} []vmopv1.NetworkDeviceStatus)",
            f"func v1a{old_num}TemplateFunctions(\n"
            f"\tnetworkStatusV1A{old_num} {old_alias}.NetworkStatus,\n"
            f"\tnetworkDevicesStatusV1A{old_num} []{old_alias}.NetworkDeviceStatus)",
            1,
        )

        # 10) Insert v1a{new_num}TemplateFunctions (full function) after v1a{old_num}TemplateFunctions
        # File uses one tab before map closing "}\n" and no tab before func "}\n"
        v1a_new_func = _make_v1aN_template_functions_block(new_num, new_ver_go)
        marker_end_prev = (
            f"\t\tconstants.{old_ver_go}FormatIP:   v1alpha{old_num}FormatIP,\n"
            "\t}\n"
            "}\n"
        )
        if v1a_new_func not in content and marker_end_prev in content:
            content = content.replace(
                marker_end_prev,
                marker_end_prev + "\n" + v1a_new_func,
                1,
            )

        self.ops.write_file(path, content)

    # -------------------------------------------------------------------------
    # Step 18: Run make generate
    # -------------------------------------------------------------------------

    def _run_make(self, target: str) -> None:
        """Run a make target with spinning progress; on failure print output."""
        if self.ops.dry_run:
            print(f"  [DRY RUN] Would run: make {target}")
            return

        result_holder: list[subprocess.CompletedProcess] = []

        def run_make() -> None:
            r = subprocess.run(
                ["make", target],
                cwd=self.ctx.root,
                capture_output=True,
                text=True,
            )
            result_holder.append(r)

        thread = threading.Thread(target=run_make)
        thread.start()

        frame = 0
        line = getattr(self, "_spinner_line", f"  Running make {target}...")
        while thread.is_alive():
            spin_char = SPINNER_CHARS[frame % len(SPINNER_CHARS)]
            sys.stdout.write(
                f"\r{line} {SPINNER_BRIGHT_WHITE}{spin_char}{SPINNER_RESET}"
            )
            sys.stdout.flush()
            time.sleep(0.1)
            frame += 1

        thread.join()
        result = result_holder[0]
        status = STATUS_OK if result.returncode == 0 else STATUS_FAIL
        sys.stdout.write(f"\r{line} {status}\n")
        sys.stdout.flush()

        if result.returncode != 0:
            if result.stdout:
                sys.stdout.write(result.stdout)
            if result.stderr:
                sys.stderr.write(result.stderr)
            sys.exit(result.returncode)

    def step_run_make_generate(self) -> None:
        """Run make generate."""
        self._run_make("generate")

    # -------------------------------------------------------------------------
    # Step 19: Run make generate-go-conversions
    # -------------------------------------------------------------------------

    def step_run_make_generate_go_conversions(self) -> None:
        """Run make generate-go-conversions."""
        self._run_make("generate-go-conversions")

    # -------------------------------------------------------------------------
    # Step 20: Run make manager-only
    # -------------------------------------------------------------------------

    def step_run_make_manager_only(self) -> None:
        """Run make manager-only."""
        self._run_make("manager-only")

    # -------------------------------------------------------------------------
    # Step 21: Run make lint-go-full
    # -------------------------------------------------------------------------

    def step_run_make_lint_go_full(self) -> None:
        """Run make lint-go-full."""
        self._run_make("lint-go-full")

    # -------------------------------------------------------------------------
    # Step 22: Run make generate-api-docs
    # -------------------------------------------------------------------------

    def step_run_make_generate_api_docs(self) -> None:
        """Run make generate-api-docs."""
        self._run_make("generate-api-docs")

    # -------------------------------------------------------------------------
    # Step 23: Update documentation
    # -------------------------------------------------------------------------

    def step_update_docs(self) -> None:
        """Update documentation with new version."""
        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        docs_dir = self.ctx.root / "docs"

        # Copy API reference doc
        old_ref = docs_dir / "ref" / "api" / f"{old_ver}.md"
        new_ref = docs_dir / "ref" / "api" / f"{new_ver}.md"
        if old_ref.exists() and not new_ref.exists():
            self.ops.copy_file(old_ref, new_ref)
            self.ops.replace_in_file(new_ref, old_ver, new_ver)

        # Update Makefile generate-api-docs
        self._update_makefile_api_docs()

        # Update mkdocs.yml
        self._update_mkdocs_nav()

        # Update docs/ref/proj/docs.md nav example
        self._update_docs_md_nav()

        # Update old hub -> new hub in all docs except docs/ref/api
        self._update_docs_content()

    def _update_makefile_api_docs(self) -> None:
        """Add generate-api-docs block for new version."""
        makefile = self.ctx.root / "Makefile"
        if not makefile.exists():
            return

        content = self.ops.read_file(makefile)
        if f"source-path=./api/{self.ctx.new_ver}" in content:
            return

        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        # Find the mv command for old version and add new block after it
        marker = f"mv ./docs/ref/api/out.md ./docs/ref/api/{old_ver}.md"
        if marker in content:
            new_block = f"""\n
\t$(CRD_REF_DOCS) \\
\t  --renderer=markdown \\
\t  --source-path=./api/{new_ver} \\
\t  --config=./.crd-ref-docs/config.yaml \\
\t  --templates-dir=./.crd-ref-docs/template \\
\t  --output-path=./docs/ref/api/
\tmv ./docs/ref/api/out.md ./docs/ref/api/{new_ver}.md"""
            content = content.replace(marker, marker + new_block)
            self.ops.write_file(makefile, content)

    def _update_mkdocs_nav(self) -> None:
        """Update mkdocs.yml navigation."""
        mkdocs_yml = self.ctx.root / "mkdocs.yml"
        if not mkdocs_yml.exists():
            return

        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        content = self.ops.read_file(mkdocs_yml)
        new_entry = f"    - {new_ver}: ref/api/{new_ver}.md"
        if new_entry in content:
            return

        old_entry = f"    - {old_ver}: ref/api/{old_ver}.md"
        if old_entry in content:
            content = content.replace(old_entry, f"{old_entry}\n{new_entry}")
            self.ops.write_file(mkdocs_yml, content)

    def _update_docs_md_nav(self) -> None:
        """Update docs/ref/proj/docs.md navigation example."""
        docs_md = self.ctx.root / "docs" / "ref" / "proj" / "docs.md"
        if not docs_md.exists():
            return

        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver

        content = self.ops.read_file(docs_md)
        new_entry = f"        - {new_ver}: ref/api/{new_ver}.md"
        if new_entry in content:
            return

        old_entry = f"        - {old_ver}: ref/api/{old_ver}.md"
        if old_entry in content:
            content = content.replace(old_entry, f"{old_entry}\n{new_entry}")
            self.ops.write_file(docs_md, content)

    def _version_go_form(self, version: str) -> str:
        """Return Go/template form of version (e.g. v1alpha5 -> V1alpha5)."""
        if not version:
            return version
        return version[0].upper() + version[1:]

    def _update_docs_content(self) -> None:
        """Update old hub version to new hub in all docs except docs/ref/api.

        docs/ref/api is excluded because it documents all API versions.
        ref/proj/docs.md is excluded because it is the nav that lists every version.
        """
        docs_dir = self.ctx.root / "docs"
        if not docs_dir.exists():
            return

        old_ver = self.ctx.old_ver
        new_ver = self.ctx.new_ver
        old_ver_go = self._version_go_form(old_ver)
        new_ver_go = self._version_go_form(new_ver)

        for doc_file in docs_dir.rglob("*"):
            if not doc_file.is_file():
                continue
            if doc_file.suffix not in [".md", ".yaml", ".yml"]:
                continue

            rel_path = str(doc_file.relative_to(docs_dir))
            # Exclude docs/ref/api (all versions live there)
            if rel_path.startswith("ref/api/") or rel_path == "ref/api":
                continue
            # Exclude nav file that lists every version
            if rel_path == "ref/proj/docs.md":
                continue

            # Replace API group version (e.g. apiVersion in YAML, prose)
            self.ops.replace_in_file(
                doc_file,
                f"vmoperator.vmware.com/{old_ver}",
                f"vmoperator.vmware.com/{new_ver}",
            )
            # Replace repo paths (e.g. GitHub links api/old_ver/... -> api/new_ver/...)
            self.ops.replace_in_file(
                doc_file,
                f"api/{old_ver}/",
                f"api/{new_ver}/",
            )
            # Replace Go/template version (e.g. .V1alpha5.Net, V1alpha5_FirstIP, "V1alpha5 variants")
            if old_ver_go != new_ver_go:
                self.ops.replace_in_file(doc_file, old_ver_go, new_ver_go)
            # Replace plain version references (e.g. "use v1alpha5" -> "use v1alpha6")
            self.ops.replace_in_file(doc_file, old_ver, new_ver)


# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Create a new API schema version for VM Operator",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s v1alpha6
  %(prog)s v1beta1 --dry-run
  %(prog)s v2 --verbose
""",
    )
    parser.add_argument(
        "new_version",
        help="New version identifier (e.g., v1alpha6, v1beta2, v2)",
    )
    parser.add_argument(
        "--dry-run",
        "-n",
        action="store_true",
        help="Show what would be done without making changes",
    )
    parser.add_argument(
        "--verbose", "-v", action="store_true", help="Show detailed output"
    )
    parser.add_argument(
        "--root",
        type=Path,
        default=None,
        help="Repository root directory (default: auto-detect)",
    )

    args = parser.parse_args()

    # Validate new version
    new_ver = args.new_version
    if not KubeVersion.validate(new_ver):
        print(
            f"Error: invalid version identifier '{new_ver}' "
            f"(expected e.g., v1alpha6, v1beta2, v2)",
            file=sys.stderr,
        )
        return 1

    # Find repository root
    if args.root:
        root = args.root.resolve()
    else:
        # Assume script is in hack/ subdirectory
        root = Path(__file__).resolve().parent.parent

    if not (root / "api").is_dir():
        print(
            f"Error: {root}/api not found. " f"Run from repository root or use --root.",
            file=sys.stderr,
        )
        return 1

    # Initialize file operations
    ops = FileOps(root, dry_run=args.dry_run, verbose=args.verbose)

    # Discover existing versions
    api_dir = root / "api"
    versions = [
        d.name
        for d in api_dir.iterdir()
        if d.is_dir() and d.name.startswith("v") and KubeVersion.validate(d.name)
    ]
    versions = sort_versions(versions)

    if not versions:
        print("Error: no existing API versions found under api/", file=sys.stderr)
        return 1

    old_ver = versions[-1]

    # Validate constraints
    if old_ver == new_ver:
        print(
            f"Error: {new_ver} is already the latest API version",
            file=sys.stderr,
        )
        return 1

    if (api_dir / new_ver).exists():
        print(f"Error: api/{new_ver} already exists", file=sys.stderr)
        return 1

    # Check that new version is actually newer
    kv_old = KubeVersion.parse(old_ver)
    kv_new = KubeVersion.parse(new_ver)
    if kv_old and kv_new and not (kv_old < kv_new):
        print(
            f"Warning: {new_ver} does not appear to be newer than {old_ver} "
            f"by Kubernetes version ordering",
            file=sys.stderr,
        )

    # Run migration
    try:
        if args.dry_run:
            print(
                f"[DRY RUN] Would create new API schema version {new_ver} "
                f"from {old_ver}..."
            )
        else:
            print(f"Creating new API schema version {new_ver} from {old_ver}...\n")

        ctx = MigrationContext(root=root, old_ver=old_ver, new_ver=new_ver, ops=ops)
        migration = SchemaMigration(ctx)
        migration.run()

        print("\n✨ Success!\n")
        print("To commit the changes, run:\n")
        print("  git add -A && git commit\n")
        return 0
    except:
        print("\n❌ Failed!\n")
        raise
    finally:
        print("To revert the changes, run:\n")
        print("  git reset --hard HEAD && git clean -fd\n")


if __name__ == "__main__":
    sys.exit(main())
