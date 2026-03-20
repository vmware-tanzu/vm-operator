# Test OVA images

This tree holds OVF/OVA bundles used by vcsim and provider tests. A **new image starts from an OVF only**; Make generates the manifest (`.mf`), the operator YAML (`.yaml`), and the `.ova` archive. Disk files are **not** checked in as full appliance images—placeholders are copied from the shared **`disk0.vmdk`** at the root of this `images/` directory (same file referenced by `ttylinux` and `uber`).

## 1. Create a directory for the image

- **Vendor / product OVFs** (typical): create a subdirectory under `vmware/`, e.g. `vmware/myproduct/`.
- **Small generic fixtures** (like `ttylinux` / `uber`): you can keep the OVF next to the top-level `Makefile` in this directory instead.

Put your **`*.ovf`** in that directory. The OVF’s `<References>` / `<File ovf:href="…">` names must match the `.vmdk` (and optional `.nvram`) filenames you place beside it.

## 2. Materialize file-backed disks from `disk0.vmdk`

Open the OVF and list every **`ovf:href` in `<References>`** that points at a **`.vmdk`**. For each distinct basename, create that file in the image directory by copying the canonical disk:

```sh
# From vmware/myproduct/ (two disks referenced in the OVF):
cp ../../disk0.vmdk myproduct-system.vmdk
cp ../../disk0.vmdk myproduct-data.vmdk
```

```sh
# From this images/ directory (top-level image, no vmware/ subdir):
cp disk0.vmdk myimage-disk0.vmdk
```

Use **exactly** the names the OVF references (including spelling and case). If the OVF also references an **`.nvram`** file, add a real or stub nvram file with that name (see `ttylinux-pc_i486-16.1` in the top-level `Makefile` for an example that includes nvram in `.mf` / `.ova`).

## 3. Add a `Makefile`

Patterns match existing bundles under `vmware/*` and the top-level `Makefile`.

**Variables**

- `IMAGE_NAME` — basename of the OVF/OVA/YAML (without extension), e.g. `myproduct-1.0.0`.
- `IMAGE_DISKS` — list of every **`.vmdk`** file that must appear in the OVA (same files as in the OVF references and your `cp` steps).
- Optional: `IMAGE_PREFIX` if the OVF name differs from disk name prefixes (see `vmware/vcsa/Makefile`).
- Optional: `IMAGE_NVRAM` if the bundle includes nvram (top-level `Makefile`).

**Targets**

1. **`$(IMAGE_NAME).mf`** — `sha1sum` of the OVF, then all disks (then nvram if any). Order must match what you pass to `tar` for the OVA.
2. **`$(IMAGE_NAME).ova`** — `tar` containing the OVF, the `.mf`, all disks, and nvram if applicable (see any existing `Makefile` here).
3. **`$(IMAGE_NAME).yaml`** — run `ovf2yaml.go` on the OVF:
   - From **`vmware/<name>/`**: `go run ../../ovf2yaml.go $(abspath $(?)) >$(@)`
   - From **this `images/` directory** (next to `ovf2yaml.go`): `go run ./ovf2yaml.go $(?) >$(@)`
4. **`build`** — depends on `$(IMAGE_NAME).ova` and `$(IMAGE_NAME).yaml`.
5. **`clean`** — remove generated `.ova`, `.yaml`, and optionally `.mf` if you treat it as generated only.

**Example skeleton** (`vmware/myproduct/Makefile`):

```makefile
ALL: build

IMAGE_NAME := myproduct-1.0.0
IMAGE_DISKS := myproduct-system.vmdk myproduct-data.vmdk

OVA := $(IMAGE_NAME).ova
YAML := $(IMAGE_NAME).yaml

$(IMAGE_NAME).mf: $(IMAGE_NAME).ovf $(IMAGE_DISKS)
	sha1sum $(^) >$(IMAGE_NAME).mf
	@cat $(@)

$(IMAGE_NAME).ova: $(IMAGE_NAME).mf
	@rm -f $(@)
	tar cvf $(@) $(IMAGE_NAME).ovf $(<) $(IMAGE_DISKS)
	tar tf $(@)

$(IMAGE_NAME).yaml: $(IMAGE_NAME).ovf
	go run ../../ovf2yaml.go $(abspath $(?)) >$(@)

build: $(OVA) $(YAML)

clean:
	rm -f $(OVA) $(YAML) $(IMAGE_NAME).mf
```

Run **`make`** or **`make build`** from that directory. You need **`go`** on `PATH` for the yaml step.

## 4. Hook into the top-level `images` build

If the image lives under **`vmware/<name>/`**, add recursive targets to the **`Makefile` in this directory** so `make build` / `make clean` from `images/` includes your subtree, e.g.:

```makefile
	$(MAKE) -C vmware/myproduct $(@)
```

## 5. Define tests (fast-deploy table entries)

Fast-deploy coverage for these OVAs lives in `pkg/providers/vsphere/vmprovider_vm_fast_deploy_test.go`, inside `FDescribeTableSubtree("images", …)`. **Add a new row by appending an `Entry`** next to the existing ones. Do not change the shared table body unless every image needs new behavior.

Each `Entry` has five arguments, in order:

1. **`name`** — Ginkgo label for the row (short, unique), e.g. `"myproduct"`.
2. **`ovfPath`** — Path to the **`.ova`** file **relative to** `test/builder/testdata/images/`, using `/` segments. Examples: `ttylinux-pc_i486-16.1.ova` (top-level) or `vmware/myproduct/myproduct-1.0.0.ova`. The test derives the YAML path by replacing `.ova` with `.yaml`, so the generated YAML must sit beside the OVA.
3. **`cachedDiskNames`** — Slice of **`.vmdk` basenames** exactly as stored in content-library storage after ingest (same strings as in the OVF `<File ovf:href="…">` references). The table matches these names to `ListLibraryItemStorage` URIs to build VMIC file IDs and to assert linked-mode `VMProvKeepDisksExtraConfigKey`. List every file-backed disk you need in that flow; order is the order used when joining names for keep-disk assertions.
4. **`numExpectedDisks`** — After a successful create, the number of `*vimtypes.VirtualDisk` devices on the VM (asserted in `createVM`). If you are unsure, run the spec once with a placeholder value and adjust from the failure, or inspect the deployed VM hardware for that OVF.
5. **`expectNvram`** — `true` if the library item must expose an **`.nvram`** file in storage and the test should require it; `false` otherwise (typical for large VMware-style bundles without nvram in the OVA).

Example (mirroring existing VMware rows):

```go
		Entry(
			"myproduct",
			"vmware/myproduct/myproduct-1.0.0.ova",
			[]string{
				"myproduct-1.0.0-system.vmdk",
				"myproduct-1.0.0-data.vmdk",
			},
			2,    // VirtualDisk count after deploy — tune to your OVF
			false,
		),
```

**vcsim / shared library:** The table **creates its own** subscribed-library item per entry from the `.ova` path, so a new `Entry` does not by itself require changes to `vcsim_test_context.go`. Update that file only if **other** tests need the same image pre-created in the default simulator library.

Run the fast deploy tests with the following command from the root of the project:

```shell
ginkgo -v -focus 'VirtualMachine FastDeploy' ./pkg/providers/vsphere
```
