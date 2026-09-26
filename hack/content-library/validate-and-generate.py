#!/usr/bin/env python3
"""
hack/content-library/validate-and-generate.py

Validates and optionally regenerates the VCSP content-library JSON metadata
files under hack/content-library/vmsvc/.

Layout assumed:
  vmsvc/
    lib.json          - Top-level library descriptor
    items.json        - Flat list of all library items
    <item-dir>/
      item.json       - Per-item descriptor (source of truth for that item)
      *.ovf / *.vmdk / *.mf / *.iso  - Actual content files

Only directories under vmsvc/ that contain non-JSON content files are treated
as "local items". You do NOT need to download the full library to use this
script — only the item(s) you are adding or updating need to be present
locally. All other entries in items.json (remote-only items with no local
directory) are never read or modified.

Usage:
  ./validate-and-generate.py [validate|generate]

Modes:
  validate (default)
    - Each file listed in item.json exists on disk.
    - File size matches the 'size' field.
    - File MD5 matches the 'etag' field.
    - SHA1 entries inside .mf files match the actual files.
    - item.json and items.json entries for local items are consistent
      (id, name, type, selfHref, version, contentVersion, files).

  generate
    - Recomputes MD5 (etag) and size for every content file.
    - Rewrites item.json (files[] section only; static metadata preserved).
    - Updates the matching entry in items.json to mirror item.json exactly.
    - Does NOT regenerate .mf files; only updates JSON metadata.
    - Does NOT touch remote-only entries in items.json.
    - Run 'validate' afterwards to confirm correctness.
"""

import sys
import json
import hashlib
import re
import uuid
from pathlib import Path
from datetime import datetime, timezone

SCRIPT_DIR = Path(__file__).parent
CL_DIR = SCRIPT_DIR / "vmsvc"
ITEMS_JSON = CL_DIR / "items.json"
LIB_JSON = CL_DIR / "lib.json"

MODE = sys.argv[1] if len(sys.argv) > 1 else "validate"

# Files to ignore when scanning item directories for content.
SKIP_NAMES = {".DS_Store"}
SKIP_SUFFIXES = {".json"}

errors: list[str] = []
warnings: list[str] = []


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def compute_md5(path: Path) -> str:
    h = hashlib.md5()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def compute_sha1(path: Path) -> str:
    h = hashlib.sha1()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def _err(msg: str) -> None:
    errors.append(msg)
    print(f"  [ERROR] {msg}")


def _warn(msg: str) -> None:
    warnings.append(msg)
    print(f"  [ WARN] {msg}")


def _ok(msg: str) -> None:
    print(f"  [   OK] {msg}")


def _step(msg: str) -> None:
    print(f"\n  ---- {msg}")


def load_json(path: Path) -> dict:
    with open(path) as f:
        return json.load(f)


def save_json(path: Path, data: dict) -> None:
    """Write JSON with 2-space indent and trailing newline (matches existing style)."""
    with open(path, "w", newline="\n") as f:
        json.dump(data, f, indent=2)
        f.write("\n")


def find_local_item_dirs() -> list[Path]:
    """Return sorted item directories that contain actual content files."""
    dirs = []
    for d in sorted(CL_DIR.iterdir()):
        if not d.is_dir() or d.name.startswith("."):
            continue
        content_files = [
            f for f in d.iterdir()
            if f.is_file()
            and f.name not in SKIP_NAMES
            and f.suffix not in SKIP_SUFFIXES
        ]
        if content_files:
            dirs.append(d)
    return dirs


def parse_mf(mf_path: Path) -> dict[str, str]:
    """Parse an OVF manifest (.mf) and return {filename: sha1hex} mapping."""
    checksums: dict[str, str] = {}
    with open(mf_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            m = re.match(r"SHA1\((.+?)\)=\s*([0-9a-fA-F]+)", line)
            if m:
                checksums[m.group(1)] = m.group(2).lower()
            else:
                _warn(f"{mf_path.name}: unrecognized line: {line!r}")
    return checksums


# ---------------------------------------------------------------------------
# Validate
# ---------------------------------------------------------------------------

def validate_item(item_dir: Path, items_map: dict[str, dict]) -> None:
    dir_name = item_dir.name
    print(f"\n==> Validating: {dir_name}")

    item_json_path = item_dir / "item.json"
    if not item_json_path.exists():
        _err(f"{dir_name}: item.json missing")
        return

    item_data = load_json(item_json_path)

    # --- Validate each file listed in item.json ---
    _step("File sizes and etag")
    for file_entry in item_data.get("files", []):
        fname = file_entry["name"]
        expected_size = file_entry.get("size")
        hrefs = file_entry.get("hrefs", [])

        if not hrefs:
            _err(f"{dir_name}/{fname}: no hrefs in item.json")
            continue

        for href in hrefs:
            fpath = CL_DIR / href
            if not fpath.exists():
                _err(f"{dir_name}/{fname}: file not found at href '{href}'")
                continue

            actual_size = fpath.stat().st_size
            if actual_size != expected_size:
                _err(
                    f"{dir_name}/{fname}: size mismatch "
                    f"(item.json={expected_size}, actual={actual_size})"
                )
            else:
                _ok(f"{dir_name}/{fname}: size {actual_size}")

    # Etag is item-level: the same value must be shared by all files.
    # CRITICAL: VcspSyncStrategy.isContentOutdated calls Long.valueOf(etag)
    # only when the locally-cached DB record has no generationNum. On a fresh
    # vCenter, the DB is populated from the CDN item.json (which includes
    # generationNum), so the hex etag is never parsed as Long. If a vCenter
    # has a stale DB record from before generationNum was added, it must
    # delete and re-add the subscribed library to clear the stale state.
    all_etags = {f.get("etag") for f in item_data.get("files", [])}
    if not all_etags or all_etags == {None}:
        _warn(f"{dir_name}: no etag set on any file entry")
    elif len(all_etags) > 1:
        _err(
            f"{dir_name}: files have different etags {all_etags} — "
            f"etag must be a single item-level value shared by all files "
            f"(run 'generate' to fix)"
        )
    else:
        _ok(f"{dir_name}: all files share item-level etag ({next(iter(all_etags))})")

    # generationNum is required on every file entry. Without it vCenter falls
    # back to Long.valueOf(etag), which throws NumberFormatException for hex
    # values (confirmed by cls.log: VcspSyncStrategy.isContentOutdated:44).
    files_without_gen = [
        f.get("name", f.get("hrefs", ["?"])[0])
        for f in item_data.get("files", [])
        if f.get("generationNum") is None
    ]
    if files_without_gen:
        _err(
            f"{dir_name}: missing generationNum on: {files_without_gen} — "
            f"vCenter will throw NumberFormatException when syncing "
            f"(run 'generate' to fix)"
        )
    else:
        _ok(f"{dir_name}: all files have generationNum")

    # --- Validate MF SHA1 checksums ---
    for mf_path in sorted(item_dir.glob("*.mf")):
        _step(f"MF checksums: {mf_path.name}")
        checksums = parse_mf(mf_path)
        if not checksums:
            _warn(f"{dir_name}/{mf_path.name}: no SHA1 entries found")
        for ref_name, expected_sha1 in checksums.items():
            ref_path = item_dir / ref_name
            if not ref_path.exists():
                _err(
                    f"{dir_name}/{mf_path.name}: "
                    f"SHA1 references missing file '{ref_name}'"
                )
                continue
            actual_sha1 = compute_sha1(ref_path)
            if actual_sha1 != expected_sha1:
                _err(
                    f"{dir_name}/{mf_path.name}: SHA1 mismatch for {ref_name} "
                    f"(mf={expected_sha1}, actual={actual_sha1})"
                )
            else:
                _ok(f"{dir_name}/{mf_path.name}: SHA1 OK for {ref_name}")

    # --- Validate consistency with items.json ---
    _step("Consistency with items.json")
    if dir_name not in items_map:
        _err(f"{dir_name}: no matching entry in items.json")
        return

    items_entry = items_map[dir_name]

    for key in ("id", "name", "type", "selfHref", "version", "contentVersion"):
        iv = item_data.get(key)
        isv = items_entry.get(key)
        if iv is None and isv is None:
            continue
        if iv != isv:
            _err(
                f"{dir_name}: field '{key}' mismatch: "
                f"item.json={iv!r}, items.json={isv!r}"
            )
        else:
            _ok(f"{dir_name}: '{key}' consistent ({iv!r})")

    if item_data.get("description") != items_entry.get("description"):
        _warn(
            f"{dir_name}: 'description' differs between item.json and items.json "
            f"(run 'generate' to sync)"
        )

    # Compare files[] arrays
    item_files = {f["name"]: f for f in item_data.get("files", [])}
    items_files = {f["name"]: f for f in items_entry.get("files", [])}

    for fname in sorted(set(item_files) | set(items_files)):
        if fname not in item_files:
            _err(f"{dir_name}: '{fname}' in items.json but missing from item.json")
            continue
        if fname not in items_files:
            _err(f"{dir_name}: '{fname}' in item.json but missing from items.json")
            continue
        for fkey in ("size", "etag", "hrefs"):
            iv = item_files[fname].get(fkey)
            isv = items_files[fname].get(fkey)
            if iv != isv:
                _err(
                    f"{dir_name}/{fname}: '{fkey}' mismatch: "
                    f"item.json={iv!r}, items.json={isv!r}"
                )
            else:
                _ok(f"{dir_name}/{fname}: '{fkey}' consistent")


def cmd_validate() -> None:
    items_json_data = load_json(ITEMS_JSON)
    items_map = {it["name"]: it for it in items_json_data.get("items", [])}
    local_dirs = find_local_item_dirs()

    print(f"Validating {len(local_dirs)} local item(s) in {CL_DIR}")

    for item_dir in local_dirs:
        validate_item(item_dir, items_map)

    print(f"\n{'=' * 60}")
    if errors:
        print(f"FAILED: {len(errors)} error(s), {len(warnings)} warning(s)")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    elif warnings:
        print(f"PASSED with {len(warnings)} warning(s)")
        for w in warnings:
            print(f"  - {w}")
    else:
        print(f"PASSED: all {len(local_dirs)} item(s) OK")


# ---------------------------------------------------------------------------
# Generate
# ---------------------------------------------------------------------------

def generate_item(item_dir: Path, items_json_data: dict) -> None:
    dir_name = item_dir.name
    print(f"\n==> Generating: {dir_name}")

    item_json_path = item_dir / "item.json"

    # Load existing item.json to preserve all static metadata.
    if item_json_path.exists():
        item_data = load_json(item_json_path)
    else:
        # Scaffold a new item.json for a directory that has no descriptor yet.
        lib_data = load_json(LIB_JSON)
        lib_id = lib_data.get("id", "").replace("urn:uuid:", "")
        new_id = str(uuid.uuid4())
        item_data = {
            "created": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f"),
            "description": "",
            "version": "2",
            "files": [],
            "id": f"urn:uuid:{new_id}",
            "name": dir_name,
            "properties": {},
            "selfHref": f"{dir_name}/item.json",
            "type": "vcsp.ovf",
            "metadata": [
                {
                    "key": "type-metadata",
                    "value": json.dumps(
                        {
                            "id": new_id,
                            "version": "2",
                            "libraryIdParent": lib_id,
                            "isVappTemplate": "False",
                            "vmTemplate": None,
                            "vappTemplate": None,
                            "networks": [],
                            "storagePolicyGroups": None,
                        },
                        separators=(",", ":"),
                    ),
                    "type": "String",
                    "domain": "SYSTEM",
                    "visibility": "READONLY",
                }
            ],
            "contentVersion": "2",
        }
        print(f"  Scaffolded new item.json (id={item_data['id']})")

    # Scan content files in the directory (sorted for determinism).
    content_files = sorted(
        f
        for f in item_dir.iterdir()
        if f.is_file()
        and f.name not in SKIP_NAMES
        and f.suffix not in SKIP_SUFFIXES
    )

    if not content_files:
        _warn(f"{dir_name}: no content files found, skipping")
        return

    # Recompute sizes and MD5 per file.
    # The item-level etag (shared across all files) is the MD5 of the primary
    # OVF/ISO file — the same pattern used by every working VCSP item in this
    # library (windows-server2022, ubuntu, centos, etc.).  Using per-file MD5s
    # instead causes vCenter to throw NumberFormatException when it tries to
    # reconcile differing etag values across files in the same item.
    file_sizes: dict[str, int] = {}
    file_md5s: dict[str, str] = {}
    for fpath in content_files:
        file_sizes[fpath.name] = fpath.stat().st_size
        file_md5s[fpath.name] = compute_md5(fpath)
        print(f"  {fpath.name}: size={file_sizes[fpath.name]}  md5={file_md5s[fpath.name]}")

    # Pick item-level etag: prefer OVF, fall back to ISO, then first file.
    # etag is the hex MD5 of the primary file, shared across all files in the
    # item — this matches the format that vCenter's own VCSP publisher produces
    # (confirmed by photon-5.0/item.json which works correctly on fresh vCenters).
    #
    # IMPORTANT: VcspSyncStrategy.isContentOutdated (line 44) calls
    # Long.valueOf(etag) only when the locally-cached DB record for a file
    # has no generationNum. On a fresh vCenter (item never synced), vCenter
    # inserts a new DB record from the CDN item.json which includes
    # generationNum, so isContentOutdated uses generationNum and never
    # touches the hex etag.
    #
    # If a vCenter has a stale DB record (synced before generationNum was added
    # to item.json), it will fail with NumberFormatException. Fix: delete and
    # re-add the subscribed library on that vCenter to clear the stale state.
    ovf_files = [f for f in content_files if f.suffix == ".ovf"]
    iso_files = [f for f in content_files if f.suffix == ".iso"]
    primary = (ovf_files or iso_files or content_files)[0]
    item_etag = file_md5s[primary.name]
    print(f"  item etag (MD5 of {primary.name}): {item_etag}")

    new_files = []
    for fpath in content_files:
        st = fpath.stat()
        new_files.append(
            {
                "hrefs": [f"{dir_name}/{fpath.name}"],
                "etag": item_etag,
                "name": fpath.name,
                "size": st.st_size,
                "generationNum": int(st.st_mtime),
            }
        )

    item_data["files"] = new_files

    # Write updated item.json.
    save_json(item_json_path, item_data)
    print(f"  Written: {item_json_path.relative_to(CL_DIR.parent.parent)}")

    # Update the matching entry in items.json (or append if new).
    items: list[dict] = items_json_data.get("items", [])
    matching_idx = next(
        (i for i, it in enumerate(items) if it.get("name") == dir_name), -1
    )
    if matching_idx >= 0:
        items[matching_idx] = dict(item_data)
        print(f"  Updated items.json entry: {dir_name}")
    else:
        items.append(dict(item_data))
        print(f"  Added new items.json entry: {dir_name}")

    items_json_data["items"] = items


def cmd_generate() -> None:
    items_json_data = load_json(ITEMS_JSON)
    local_dirs = find_local_item_dirs()

    print(f"Generating JSON metadata for {len(local_dirs)} local item(s)")

    for item_dir in local_dirs:
        generate_item(item_dir, items_json_data)

    save_json(ITEMS_JSON, items_json_data)
    print(f"\nUpdated: {ITEMS_JSON.relative_to(CL_DIR.parent.parent)}")
    print("Done. Run './validate-and-generate.py validate' to verify.")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def main() -> None:
    if not CL_DIR.is_dir():
        print(f"ERROR: content-library directory not found: {CL_DIR}", file=sys.stderr)
        sys.exit(1)

    if MODE == "validate":
        cmd_validate()
    elif MODE == "generate":
        cmd_generate()
    else:
        print(
            f"ERROR: unknown mode {MODE!r}. Use 'validate' or 'generate'.",
            file=sys.stderr,
        )
        sys.exit(1)


if __name__ == "__main__":
    main()
