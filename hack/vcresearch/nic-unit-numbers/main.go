// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Command nic-unit-numbers characterises how vSphere assigns, honours, and
// preserves VirtualDevice.UnitNumber on virtual ethernet cards. It exists to
// answer the open questions in .sdd/specs/006-nic-unit-numbers/spec.md (Q1-Q7)
// against a real vCenter; the answers gate the NIC unit numbers feature.
//
// The field of concern is VirtualDevice.UnitNumber, NOT the PCI slot number
// (VirtualDevice.SlotInfo / VirtualDevicePciBusSlotInfo.PciSlotNumber). The
// PCI slot number is recorded alongside every observation because it is cheap
// and occasionally illuminating, but no product behaviour depends on it.
//
// VirtualDevice.UnitNumber is a *int32 in govmomi: nil means "let the platform
// assign". The API never sends -1, so there are no vSphere-side -1 semantics to
// test; the nil case is what every "no explicit unit number" experiment below
// exercises.
//
// E16 (suspend/resume), E17 (vMotion), E18 (hardware-version upgrade), and
// E19 (an explicit unit number outside the 7-16 band) are not among the 15
// experiments tasks.md T001 originally enumerated, and their questions are
// not part of spec.md's Q1-Q8 set. They were added after the initial
// research run, on request, to also characterise those lifecycle
// transitions and platform behaviours; treat their results as informational
// context rather than a resolution of any spec.md open question.
//
// The program creates VMs, so point it at a scratch environment. Every VM it
// creates is destroyed on exit unless -keep is passed.
//
// Usage:
//
//	go run ./hack/vcresearch/nic-unit-numbers \
//	  -url 'https://user:pass@vcenter.example.com/sdk' -insecure \
//	  -datacenter DC0 -pool /DC0/host/C0/Resources -datastore ds0 \
//	  -network /DC0/network/VM-Network \
//	  -out research-results.md
//
// Connection and placement flags fall back to the standard GOVC_* environment
// variables, so an environment already set up for govc needs only -out.
//
// This program is disposable: .sdd/specs/006-nic-unit-numbers/tasks.md T023
// deletes it once research.md records the findings.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// nicUnitNumberFirst and nicUnitNumberLast bound the unit-number band the
// platform statically allocates to ethernet cards on the virtual PCI bus. See
// external/vim/api/v1alpha1/testdata/device_keys.txt: ethernet NICs own
// pci:7..16, VMCI owns 17, and PCI passthrough owns 18..21 and 38..161.
const (
	nicUnitNumberFirst = int32(7)
	nicUnitNumberLast  = int32(16)
)

// Experiment IDs. These map one-to-one onto the numbered experiments in
// tasks.md T001, so a result here can be traced back to the item that asked
// for it.
const (
	e01CreateVMExplicit    = "E01"
	e02DeployOVFExplicit   = "E02"
	e03ReconfigureExplicit = "E03"
	e04ReconfigureAuto     = "E04"
	e05AddRemove           = "E05"
	e06SameSlotReuse       = "E06"
	e07EditUnitNumber      = "E07"
	e08Collision           = "E08"
	e09ControllerKey       = "E09"
	e10NonNICPCIOccupant   = "E10"
	e11FreedSlotReuse      = "E11"
	e12PowerCycleStability = "E12"
	e13OutOfBandAdd        = "E13"
	e14SRIOV               = "E14"
	e15HotAddExplicit      = "E15"

	// E16, E17, and E18 are not in tasks.md T001's original 15-item list,
	// and none of these lifecycle transitions appear in spec.md's Q1-Q8.
	// Added to characterise VM lifecycle transitions the original scope
	// omitted.
	e16SuspendResume          = "E16"
	e17VMotion                = "E17"
	e18HardwareVersionUpgrade = "E18"

	// E19 is also not in tasks.md T001's original 15-item list. It answers
	// a question the design otherwise assumes rather than observes: what
	// does vSphere itself do with an explicit UnitNumber outside the
	// platform's 7-16 ethernet-card band, on both CreateVM and a
	// ReconfigVM_Task Add? The webhook is expected to reject this before it
	// ever reaches vSphere (tasks.md T013), so this experiment is about the
	// platform's own behaviour as a defense-in-depth/backfill-skip check
	// (spec.md G7, G9), not about changing that webhook design.
	e19OutOfRangeUnitNumber = "E19"
)

// outOfRangeUnits are UnitNumber values this program requests to see how
// vSphere treats a value outside the ethernet-card band (7-16). Chosen to
// probe distinct static PCI-unit bands (device_keys.txt): 3 sits in the SCSI
// HBA band, 17 is VMCI's own reserved unit, and 200 has no PCI-unit meaning
// at all on this platform.
var outOfRangeUnits = []int32{3, 17, 200}

// Result statuses.
const (
	statusHonoured    = "HONOURED"
	statusNotHonoured = "NOT HONOURED"
	statusRecorded    = "RECORDED"
	statusSkipped     = "SKIPPED"
	statusError       = "ERROR"
)

const defaultEthernetCardType = "vmxnet3"

// config holds everything the program needs to reach a vCenter and place VMs.
type config struct {
	vcURL    string
	username string
	password string
	insecure bool

	datacenter   string
	resourcePool string
	datastore    string
	folder       string
	network      string

	// libraryItem names or IDs an OVF-type content library item. Required by
	// E02; without it that experiment records an explicit skip.
	libraryItem string

	// sriovNetwork names an SR-IOV-capable network. Required by E14.
	sriovNetwork string
	// sriovPhysicalFunction pins a specific physical function. When empty,
	// the automatic assignment sentinel is used instead.
	sriovPhysicalFunction string

	// vGPUProfile and dvxDeviceClass each supply a non-NIC PCI-bus occupant
	// for E10. Either one is enough; without both that experiment skips.
	vGPUProfile    string
	dvxDeviceClass string

	hardwareVersion string
	guestID         string
	vmPrefix        string
	supportMatrix   string

	// e18StartVersion is the hardware version E18 explicitly requests at
	// create time, before upgrading it. It must be older than whatever this
	// environment's host(s) currently support, or E18 records a skip
	// (AlreadyUpgraded) instead of an upgrade. A flag rather than a
	// hardcoded constant because that "older than" requirement is
	// environment-dependent — see e18HardwareVersionUpgradeDefault.
	e18StartVersion string

	// e18TargetVersions is the ordered, comma-separated list of hardware
	// versions E18 upgrades the VM through, one UpgradeVM_Task per entry
	// (each followed by a power-on/power-off cycle at that version before
	// moving to the next). The literal token "max" stands in for
	// UpgradeVM_Task's empty-string "upgrade to the host's latest
	// supported" target.
	e18TargetVersions string

	only        string
	skip        string
	interactive bool
	keep        bool

	outMarkdown string
	outJSON     string

	taskTimeout time.Duration
}

// envOr returns the value of the first non-empty environment variable named in
// keys, or def when none is set.
func envOr(def string, keys ...string) string {
	for _, k := range keys {
		v := os.Getenv(k)
		if v != "" {
			return v
		}
	}

	return def
}

// registerFlags binds cfg to the command line, defaulting to the GOVC_*
// environment variables so an environment already configured for govc works
// without repeating itself.
func registerFlags(fs *flag.FlagSet, cfg *config) {
	fs.StringVar(&cfg.vcURL, "url", envOr("", "VC_URL", "GOVC_URL"),
		"vCenter SDK URL, e.g. https://vcenter.example.com/sdk (env: VC_URL, GOVC_URL)")
	fs.StringVar(&cfg.username, "username", envOr("", "VC_USERNAME", "GOVC_USERNAME"),
		"vCenter username (env: VC_USERNAME, GOVC_USERNAME)")
	fs.StringVar(&cfg.password, "password", envOr("", "VC_PASSWORD", "GOVC_PASSWORD"),
		"vCenter password (env: VC_PASSWORD, GOVC_PASSWORD)")
	fs.BoolVar(&cfg.insecure, "insecure", envOr("false", "VC_INSECURE", "GOVC_INSECURE") == "true",
		"skip vCenter TLS certificate verification (env: VC_INSECURE, GOVC_INSECURE)")

	fs.StringVar(&cfg.datacenter, "datacenter", envOr("", "GOVC_DATACENTER"),
		"datacenter inventory path or name (env: GOVC_DATACENTER)")
	fs.StringVar(&cfg.resourcePool, "pool", envOr("", "GOVC_RESOURCE_POOL"),
		"resource pool inventory path (env: GOVC_RESOURCE_POOL)")
	fs.StringVar(&cfg.datastore, "datastore", envOr("", "GOVC_DATASTORE"),
		"datastore name or path (env: GOVC_DATASTORE)")
	fs.StringVar(&cfg.folder, "folder", envOr("", "GOVC_FOLDER"),
		"VM folder inventory path (env: GOVC_FOLDER)")
	fs.StringVar(&cfg.network, "network", envOr("", "GOVC_NETWORK"),
		"network to back the research NICs (env: GOVC_NETWORK)")

	fs.StringVar(&cfg.libraryItem, "library-item", "",
		"name or ID of an OVF content library item; required by "+e02DeployOVFExplicit)
	fs.StringVar(&cfg.sriovNetwork, "sriov-network", "",
		"SR-IOV capable network; required by "+e14SRIOV)
	fs.StringVar(&cfg.sriovPhysicalFunction, "sriov-physical-function", "",
		"SR-IOV physical function ID; defaults to automatic assignment")
	fs.StringVar(&cfg.vGPUProfile, "vgpu-profile", "",
		"vGPU profile for a VirtualPCIPassthrough vmiop device; used by "+e10NonNICPCIOccupant)
	fs.StringVar(&cfg.dvxDeviceClass, "dvx-device-class", "",
		"DVX device class for a VirtualPCIPassthrough dvx device; used by "+e10NonNICPCIOccupant)

	fs.StringVar(&cfg.hardwareVersion, "hardware-version", "",
		"VM hardware version, e.g. vmx-21; empty lets vCenter choose")
	fs.StringVar(&cfg.e18StartVersion, "e18-start-version", e18HardwareVersionUpgradeDefault,
		"hardware version E18 creates the VM at before upgrading it; must be "+
			"older than the environment's max, or E18 records a skip; used by "+e18HardwareVersionUpgrade)
	fs.StringVar(&cfg.e18TargetVersions, "e18-target-versions", "vmx-17,vmx-20,max",
		"comma-separated, ordered hardware versions E18 upgrades the VM through, one "+
			"UpgradeVM_Task and power-on/power-off cycle per entry; \"max\" (case-insensitive) "+
			"means UpgradeVM_Task's empty-string \"host's latest supported\" target; used by "+
			e18HardwareVersionUpgrade)
	fs.StringVar(&cfg.guestID, "guest-id", "otherGuest64", "guest ID for the research VMs")
	fs.StringVar(&cfg.vmPrefix, "vm-prefix", "nic-unit-research", "name prefix for the research VMs")
	fs.StringVar(&cfg.supportMatrix, "support-matrix", "",
		"free-form note naming the support matrix this run is assumed to cover (R6)")

	fs.StringVar(&cfg.only, "only", "", "comma-separated experiment IDs to run, e.g. E01,E06")
	fs.StringVar(&cfg.skip, "skip", "", "comma-separated experiment IDs to skip")
	fs.BoolVar(&cfg.interactive, "interactive", false,
		"pause for manual vCenter UI steps; required by "+e13OutOfBandAdd)
	fs.BoolVar(&cfg.keep, "keep", false, "do not destroy the research VMs on exit")

	fs.StringVar(&cfg.outMarkdown, "out", "", "write the Markdown report to this file instead of stdout")
	fs.StringVar(&cfg.outJSON, "out-json", "", "also write the raw results as JSON to this file")

	fs.DurationVar(&cfg.taskTimeout, "task-timeout", 10*time.Minute, "per-vSphere-task timeout")
}

// deviceInfo is a flattened view of a virtual device, recorded for both the
// requested payload and the observed hardware so the two can be compared.
type deviceInfo struct {
	Kind          string `json:"kind"`
	Key           int32  `json:"key"`
	UnitNumber    *int32 `json:"unitNumber"`
	ControllerKey int32  `json:"controllerKey"`
	PCISlotNumber *int32 `json:"pciSlotNumber,omitempty"`
	MACAddress    string `json:"macAddress,omitempty"`
	AddressType   string `json:"addressType,omitempty"`
	ExternalID    string `json:"externalID,omitempty"`
	Backing       string `json:"backing,omitempty"`
}

// unit renders the unit number for display, distinguishing an explicit value
// from "the payload asked the platform to choose".
func (d deviceInfo) unit() string {
	if d.UnitNumber == nil {
		return "nil (auto)"
	}

	return strconv.Itoa(int(*d.UnitNumber))
}

// String renders one device as a single report line.
func (d deviceInfo) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "unit=%s key=%d controllerKey=%d kind=%s", d.unit(), d.Key, d.ControllerKey, d.Kind)

	if d.PCISlotNumber != nil {
		fmt.Fprintf(&b, " pciSlot=%d", *d.PCISlotNumber)
	}

	if d.MACAddress != "" {
		fmt.Fprintf(&b, " mac=%s/%s", d.MACAddress, d.AddressType)
	}

	if d.ExternalID != "" {
		fmt.Fprintf(&b, " externalID=%s", d.ExternalID)
	}

	if d.Backing != "" {
		fmt.Fprintf(&b, " backing=%s", d.Backing)
	}

	return b.String()
}

// faultInfo records the detail of a vSphere fault. Q4 turns on the fault type,
// so the type name, the localized message, and InvalidDeviceSpec's deviceIndex
// are all captured rather than just the error string (R7).
type faultInfo struct {
	Type        string   `json:"type"`
	Message     string   `json:"message,omitempty"`
	Property    string   `json:"property,omitempty"`
	DeviceIndex *int32   `json:"deviceIndex,omitempty"`
	Localized   []string `json:"localized,omitempty"`
}

// step is one operation within an experiment, recording what was asked for
// against what the platform actually did (R7).
type step struct {
	Name      string       `json:"name"`
	Requested []deviceInfo `json:"requested,omitempty"`
	Observed  []deviceInfo `json:"observed,omitempty"`
	Faults    []faultInfo  `json:"faults,omitempty"`
	Err       string       `json:"error,omitempty"`
	Notes     []string     `json:"notes,omitempty"`
}

// result is the record for one experiment.
type result struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Questions []string `json:"questions,omitempty"`
	Status    string   `json:"status"`
	Reason    string   `json:"reason,omitempty"`
	Steps     []step   `json:"steps,omitempty"`
	Findings  []string `json:"findings,omitempty"`
}

// skip marks the experiment as not run and records why. A missing environment
// prerequisite is recorded explicitly rather than omitted silently (R5).
func (r *result) skip(reason string) *result {
	r.Status = statusSkipped
	r.Reason = reason

	return r
}

// fail marks the experiment as errored and records the error.
func (r *result) fail(err error) *result {
	r.Status = statusError
	r.Reason = err.Error()

	return r
}

// find returns a finding line.
func (r *result) find(format string, args ...any) {
	r.Findings = append(r.Findings, fmt.Sprintf(format, args...))
}

// environment records the platform this run characterises. A single-vCenter
// run cannot answer the cross-version stability question on its own, so the
// exact builds are recorded with every report (R6).
type environment struct {
	RunAt           string `json:"runAt"`
	VCName          string `json:"vcName"`
	VCVersion       string `json:"vcVersion"`
	VCBuild         string `json:"vcBuild"`
	VCAPIVersion    string `json:"vcAPIVersion"`
	HostName        string `json:"hostName,omitempty"`
	HostVersion     string `json:"hostVersion,omitempty"`
	HostBuild       string `json:"hostBuild,omitempty"`
	HardwareVersion string `json:"hardwareVersion,omitempty"`
	Datacenter      string `json:"datacenter"`
	ResourcePool    string `json:"resourcePool"`
	Datastore       string `json:"datastore"`
	Folder          string `json:"folder"`
	Network         string `json:"network"`
	SupportMatrix   string `json:"supportMatrix,omitempty"`
	GovmomiVersion  string `json:"govmomiVersion"`
}

// report is the whole run: the environment plus every experiment result.
type report struct {
	Environment environment `json:"environment"`
	Results     []*result   `json:"results"`
}

// renderMarkdown renders the report as a Markdown section suitable for pasting
// into .sdd/specs/006-nic-unit-numbers/research.md.
func (r report) renderMarkdown() string {
	var b strings.Builder

	b.WriteString("# NIC unit numbers — govmomi research results (T001)\n\n")
	b.WriteString("## Environment\n\n")
	b.WriteString("| Property | Value |\n|---|---|\n")

	env := r.Environment
	rows := [][2]string{
		{"Run at", env.RunAt},
		{"vCenter", env.VCName},
		{"vCenter version", env.VCVersion},
		{"vCenter build", env.VCBuild},
		{"vCenter API version", env.VCAPIVersion},
		{"ESX host", env.HostName},
		{"ESX version", env.HostVersion},
		{"ESX build", env.HostBuild},
		{"VM hardware version", env.HardwareVersion},
		{"Datacenter", env.Datacenter},
		{"Resource pool", env.ResourcePool},
		{"Datastore", env.Datastore},
		{"Folder", env.Folder},
		{"Network", env.Network},
		{"Support matrix covered", env.SupportMatrix},
		{"govmomi", env.GovmomiVersion},
	}

	for _, row := range rows {
		v := row[1]
		if v == "" {
			v = "_(not recorded)_"
		}

		fmt.Fprintf(&b, "| %s | %s |\n", row[0], v)
	}

	b.WriteString("\n> A single-vCenter run does not answer cross-version stability. ")
	b.WriteString("Treat every result below as characterising the builds named above only (R6).\n")

	b.WriteString("\n## Summary\n\n")
	b.WriteString("| Experiment | Question(s) | Status | Title |\n|---|---|---|---|\n")

	for _, res := range r.Results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			res.ID, strings.Join(res.Questions, ", "), res.Status, res.Title)
	}

	b.WriteString("\n## Results\n")

	for _, res := range r.Results {
		res.renderMarkdown(&b)
	}

	return b.String()
}

// renderMarkdown appends one experiment's record to b.
func (r *result) renderMarkdown(b *strings.Builder) {
	fmt.Fprintf(b, "\n### %s — %s\n\n", r.ID, r.Title)

	if len(r.Questions) > 0 {
		fmt.Fprintf(b, "**Answers**: %s\n\n", strings.Join(r.Questions, ", "))
	}

	fmt.Fprintf(b, "**Status**: %s\n", r.Status)

	if r.Reason != "" {
		fmt.Fprintf(b, "\n**Reason**: %s\n", r.Reason)
	}

	for i := range r.Steps {
		r.Steps[i].renderMarkdown(b)
	}

	if len(r.Findings) > 0 {
		b.WriteString("\n**Findings**:\n\n")

		for _, f := range r.Findings {
			fmt.Fprintf(b, "- %s\n", f)
		}
	}
}

// renderMarkdown appends one step's record to b.
func (s step) renderMarkdown(b *strings.Builder) {
	fmt.Fprintf(b, "\n#### Step: %s\n", s.Name)

	renderDevices(b, "Requested", s.Requested)
	renderDevices(b, "Observed", s.Observed)

	if s.Err != "" {
		fmt.Fprintf(b, "\nError: `%s`\n", s.Err)
	}

	for _, f := range s.Faults {
		fmt.Fprintf(b, "\nFault: `%s`", f.Type)

		if f.DeviceIndex != nil {
			fmt.Fprintf(b, " deviceIndex=`%d`", *f.DeviceIndex)
		}

		if f.Property != "" {
			fmt.Fprintf(b, " property=`%s`", f.Property)
		}

		if f.Message != "" {
			fmt.Fprintf(b, "\n\n> %s", f.Message)
		}

		for _, m := range f.Localized {
			fmt.Fprintf(b, "\n>\n> %s", m)
		}

		b.WriteString("\n")
	}

	for _, n := range s.Notes {
		fmt.Fprintf(b, "\n%s\n", n)
	}
}

// renderDevices appends a labelled device block, or nothing when empty.
func renderDevices(b *strings.Builder, label string, devices []deviceInfo) {
	if len(devices) == 0 {
		return
	}

	fmt.Fprintf(b, "\n%s:\n\n```\n", label)

	for _, d := range devices {
		fmt.Fprintf(b, "%s\n", d)
	}

	b.WriteString("```\n")
}

// runner holds the connected clients and the resolved inventory the
// experiments place VMs into.
type runner struct {
	cfg    config
	client *govmomi.Client
	rest   *rest.Client
	finder *find.Finder

	datacenter   *object.Datacenter
	resourcePool *object.ResourcePool
	datastore    *object.Datastore
	folder       *object.Folder
	network      object.NetworkReference

	env     environment
	results []*result

	// created tracks every VM the program created so cleanup can destroy
	// them all, including those belonging to a failed experiment (R7).
	created []*object.VirtualMachine
	runID   string
}

// connect logs in to vCenter over both SOAP and REST and resolves the
// inventory objects the experiments need.
func (r *runner) connect(ctx context.Context) error {
	u, err := soapURL(r.cfg)
	if err != nil {
		return err
	}

	r.client, err = govmomi.NewClient(ctx, u, r.cfg.insecure)
	if err != nil {
		return fmt.Errorf("failed to connect to vCenter: %w", err)
	}

	r.rest = rest.NewClient(r.client.Client)

	err = r.rest.Login(ctx, u.User)
	if err != nil {
		return fmt.Errorf("failed to log in to the vCenter REST endpoint: %w", err)
	}

	r.finder = find.NewFinder(r.client.Client, true)

	r.datacenter, err = r.finder.DatacenterOrDefault(ctx, r.cfg.datacenter)
	if err != nil {
		return fmt.Errorf("failed to find datacenter %q: %w", r.cfg.datacenter, err)
	}

	r.finder.SetDatacenter(r.datacenter)

	r.resourcePool, err = r.finder.ResourcePoolOrDefault(ctx, r.cfg.resourcePool)
	if err != nil {
		return fmt.Errorf("failed to find resource pool %q: %w", r.cfg.resourcePool, err)
	}

	r.datastore, err = r.finder.DatastoreOrDefault(ctx, r.cfg.datastore)
	if err != nil {
		return fmt.Errorf("failed to find datastore %q: %w", r.cfg.datastore, err)
	}

	r.folder, err = r.finder.FolderOrDefault(ctx, r.cfg.folder)
	if err != nil {
		return fmt.Errorf("failed to find folder %q: %w", r.cfg.folder, err)
	}

	r.network, err = r.finder.NetworkOrDefault(ctx, r.cfg.network)
	if err != nil {
		return fmt.Errorf("failed to find network %q: %w", r.cfg.network, err)
	}

	return nil
}

// soapURL builds the vCenter SDK URL, folding in the username and password
// flags when the URL does not already carry credentials.
func soapURL(cfg config) (*url.URL, error) {
	if cfg.vcURL == "" {
		return nil, errors.New("no vCenter URL: pass -url or set VC_URL/GOVC_URL")
	}

	raw := cfg.vcURL
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vCenter URL %q: %w", cfg.vcURL, err)
	}

	if u.Path == "" || u.Path == "/" {
		u.Path = "/sdk"
	}

	if u.User == nil && cfg.username != "" {
		u.User = url.UserPassword(cfg.username, cfg.password)
	}

	if u.User == nil {
		return nil, errors.New("no vCenter credentials: pass -username/-password or embed them in -url")
	}

	return u, nil
}

// recordEnvironment captures the vCenter and ESX builds this run
// characterises, plus the inventory it used (R6).
func (r *runner) recordEnvironment(ctx context.Context) {
	about := r.client.ServiceContent.About

	r.env = environment{
		RunAt:           time.Now().UTC().Format(time.RFC3339),
		VCName:          fmt.Sprintf("%s (%s)", r.client.URL().Host, about.FullName),
		VCVersion:       about.Version,
		VCBuild:         about.Build,
		VCAPIVersion:    about.ApiVersion,
		HardwareVersion: r.cfg.hardwareVersion,
		Datacenter:      r.datacenter.InventoryPath,
		ResourcePool:    r.resourcePool.InventoryPath,
		Datastore:       r.datastore.InventoryPath,
		Folder:          r.folder.InventoryPath,
		Network:         r.network.Reference().String(),
		SupportMatrix:   r.cfg.supportMatrix,
		GovmomiVersion:  govmomiVersion(),
	}

	if r.cfg.network != "" {
		r.env.Network = r.cfg.network
	}

	hosts, err := r.finder.HostSystemList(ctx, "*")
	if err != nil || len(hosts) == 0 {
		return
	}

	var moHost mo.HostSystem

	err = hosts[0].Properties(ctx, hosts[0].Reference(), []string{"name", "config.product"}, &moHost)
	if err != nil {
		return
	}

	r.env.HostName = moHost.Name

	if moHost.Config != nil {
		r.env.HostVersion = moHost.Config.Product.Version
		r.env.HostBuild = moHost.Config.Product.Build
	}
}

// govmomiVersion reports the govmomi module version this program was built
// against, which is the same version the product is pinned to.
func govmomiVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	for _, dep := range info.Deps {
		if dep.Path == "github.com/vmware/govmomi" {
			return dep.Version
		}
	}

	return "unknown"
}

// deviceInfoFor flattens a virtual device into the record the report uses.
func deviceInfoFor(dev vimtypes.BaseVirtualDevice) deviceInfo {
	vd := dev.GetVirtualDevice()

	di := deviceInfo{
		Kind:          typeName(dev),
		Key:           vd.Key,
		UnitNumber:    copyInt32(vd.UnitNumber),
		ControllerKey: vd.ControllerKey,
		Backing:       backingSummary(vd.Backing),
	}

	slot, ok := vd.SlotInfo.(*vimtypes.VirtualDevicePciBusSlotInfo)
	if ok {
		di.PCISlotNumber = ptr.To(slot.PciSlotNumber)
	}

	eth, ok := dev.(vimtypes.BaseVirtualEthernetCard)
	if ok {
		card := eth.GetVirtualEthernetCard()
		di.MACAddress = card.MacAddress
		di.AddressType = card.AddressType
		di.ExternalID = card.ExternalId
	}

	return di
}

// copyInt32 returns a copy of the pointed-to value so the record does not
// alias a field inside a managed object that may be refetched.
func copyInt32(v *int32) *int32 {
	if v == nil {
		return nil
	}

	return ptr.To(*v)
}

// typeName returns the concrete VIM type name of v, e.g. VirtualVmxnet3.
func typeName(v any) string {
	if v == nil {
		return ""
	}

	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.Name()
}

// backingSummary renders just enough of a device backing to tell two devices
// apart in the report.
func backingSummary(backing vimtypes.BaseVirtualDeviceBackingInfo) string {
	switch b := backing.(type) {
	case nil:
		return ""
	case *vimtypes.VirtualEthernetCardNetworkBackingInfo:
		return fmt.Sprintf("%s(%s)", typeName(b), b.DeviceName)
	case *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo:
		return fmt.Sprintf("%s(portgroup=%s port=%s)", typeName(b), b.Port.PortgroupKey, b.Port.PortKey)
	case *vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo:
		return fmt.Sprintf("%s(%s/%s)", typeName(b), b.OpaqueNetworkType, b.OpaqueNetworkId)
	case *vimtypes.VirtualPCIPassthroughVmiopBackingInfo:
		return fmt.Sprintf("%s(vgpu=%s)", typeName(b), b.Vgpu)
	case *vimtypes.VirtualPCIPassthroughDvxBackingInfo:
		return fmt.Sprintf("%s(deviceClass=%s)", typeName(b), b.DeviceClass)
	default:
		return typeName(b)
	}
}

// observeEthCards returns the VM's ethernet cards in hardware order.
func (r *runner) observeEthCards(ctx context.Context, vm *object.VirtualMachine) ([]deviceInfo, error) {
	devices, err := r.hardware(ctx, vm)
	if err != nil {
		return nil, err
	}

	return infosFor(devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))), nil
}

// observePCIBus returns every device the platform placed on the virtual PCI
// bus: ethernet cards, PCI passthrough devices, and the VMCI device. E10 needs
// the whole bus, not only the NICs.
func (r *runner) observePCIBus(ctx context.Context, vm *object.VirtualMachine) ([]deviceInfo, error) {
	devices, err := r.hardware(ctx, vm)
	if err != nil {
		return nil, err
	}

	controllers := devices.SelectByType((*vimtypes.VirtualPCIController)(nil))
	keys := make([]int32, 0, len(controllers))

	for _, c := range controllers {
		keys = append(keys, c.GetVirtualDevice().Key)
	}

	var onBus []vimtypes.BaseVirtualDevice

	for _, d := range devices {
		if slices.Contains(keys, d.GetVirtualDevice().ControllerKey) {
			onBus = append(onBus, d)
		}
	}

	infos := infosFor(onBus)
	slices.SortStableFunc(infos, func(a, b deviceInfo) int {
		return unitOrder(a) - unitOrder(b)
	})

	return infos, nil
}

// unitOrder sorts devices by unit number, placing unset units last.
func unitOrder(d deviceInfo) int {
	if d.UnitNumber == nil {
		return 1 << 20
	}

	return int(*d.UnitNumber)
}

// infosFor flattens a device list into report records.
func infosFor(devices []vimtypes.BaseVirtualDevice) []deviceInfo {
	infos := make([]deviceInfo, 0, len(devices))

	for _, d := range devices {
		infos = append(infos, deviceInfoFor(d))
	}

	return infos
}

// hardware refetches the VM's device list from vCenter. Every observation goes
// through a fresh read so a stale cache cannot be mistaken for a stable slot.
func (r *runner) hardware(
	ctx context.Context, vm *object.VirtualMachine) (object.VirtualDeviceList, error) {

	var moVM mo.VirtualMachine

	err := vm.Properties(ctx, vm.Reference(), []string{"config.hardware.device", "config.version"}, &moVM)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch hardware for %s: %w", vm.Reference().Value, err)
	}

	// Record the hardware version vCenter actually assigned, not merely the
	// one requested (R6). Only the first observation sticks, which means this
	// records whichever experiment happens to run first's version, NOT
	// necessarily every experiment's — e.g. an OVF deploy's VM can come up at
	// the version the descriptor declares, which may differ from a directly
	// created VM's default. If per-experiment version matters, read it from
	// each experiment's own step rather than trusting this single field.
	if moVM.Config != nil && r.env.HardwareVersion == "" {
		r.env.HardwareVersion = moVM.Config.Version
	}

	if moVM.Config == nil {
		return nil, fmt.Errorf("VM %s has no config", vm.Reference().Value)
	}

	return object.VirtualDeviceList(moVM.Config.Hardware.Device), nil
}

// captureFaults walks err and records every vSphere fault it carries, so a
// collision result names the fault type rather than just its message (R7, Q4).
func captureFaults(err error) []faultInfo {
	if err == nil {
		return nil
	}

	var infos []faultInfo

	fault.In(err, func(
		f vimtypes.BaseMethodFault,
		localized string,
		msgs []vimtypes.LocalizableMessage) bool {

		info := faultInfo{
			Type:    typeName(f),
			Message: localized,
		}

		switch tf := f.(type) {
		case *vimtypes.InvalidDeviceSpec:
			info.Property = tf.Property
			info.DeviceIndex = ptr.To(tf.DeviceIndex)
		case *vimtypes.InvalidVmConfig:
			info.Property = tf.Property
		}

		for _, m := range msgs {
			info.Localized = append(info.Localized, fmt.Sprintf("%s: %s", m.Key, m.Message))
		}

		infos = append(infos, info)

		return true
	})

	return infos
}

// vmName builds a unique, traceable name for a research VM.
func (r *runner) vmName(experiment, suffix string) string {
	name := fmt.Sprintf("%s-%s-%s", r.cfg.vmPrefix, strings.ToLower(experiment), r.runID)
	if suffix != "" {
		name += "-" + suffix
	}

	return name
}

// baseConfigSpec returns a minimal ConfigSpec for a diskless research VM.
func (r *runner) baseConfigSpec(name string) vimtypes.VirtualMachineConfigSpec {
	return vimtypes.VirtualMachineConfigSpec{
		Name:     name,
		GuestId:  r.cfg.guestID,
		Version:  r.cfg.hardwareVersion,
		NumCPUs:  1,
		MemoryMB: 512,
		Files: &vimtypes.VirtualMachineFileInfo{
			VmPathName: fmt.Sprintf("[%s]", r.datastore.Name()),
		},
	}
}

// newEthCard builds an ethernet card the same way the operator does:
// object.EthernetCardTypes().CreateEthernetCard with the network's backing,
// leaving ControllerKey unset. E09 turns on that last detail, so no experiment
// may set ControllerKey without saying so.
func (r *runner) newEthCard(
	ctx context.Context,
	net object.NetworkReference,
	unitNumber *int32) (vimtypes.BaseVirtualDevice, error) {

	backing, err := net.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ethernet card backing for %v: %w", net.Reference(), err)
	}

	dev, err := object.EthernetCardTypes().CreateEthernetCard(defaultEthernetCardType, backing)
	if err != nil {
		return nil, fmt.Errorf("failed to create ethernet card: %w", err)
	}

	card := dev.(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
	card.AddressType = string(vimtypes.VirtualEthernetCardMacTypeGenerated)
	card.GetVirtualDevice().UnitNumber = unitNumber

	return dev, nil
}

// newSriovCard builds a VirtualSriovEthernetCard, using automatic physical
// function assignment unless a specific PF was named.
func (r *runner) newSriovCard(
	ctx context.Context,
	net object.NetworkReference,
	unitNumber *int32) (vimtypes.BaseVirtualDevice, error) {

	backing, err := net.EthernetCardBackingInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ethernet card backing for %v: %w", net.Reference(), err)
	}

	pf := r.cfg.sriovPhysicalFunction
	if pf == "" {
		// The sentinel that asks vSphere to pick a physical function from the
		// network's SR-IOV device pool.
		pf = "Automatic-0000:00:00.0"
	}

	card := &vimtypes.VirtualSriovEthernetCard{
		VirtualEthernetCard: vimtypes.VirtualEthernetCard{
			VirtualDevice: vimtypes.VirtualDevice{
				Key:        -100,
				Backing:    backing,
				UnitNumber: unitNumber,
			},
			AddressType: string(vimtypes.VirtualEthernetCardMacTypeGenerated),
		},
		SriovBacking: &vimtypes.VirtualSriovEthernetCardSriovBackingInfo{
			PhysicalFunctionBacking: &vimtypes.VirtualPCIPassthroughDeviceBackingInfo{
				Id: pf,
			},
		},
	}

	return card, nil
}

// newPCIPassthrough builds a non-NIC PCI-bus occupant for E10, from whichever
// of the two backings the environment supplied.
func (r *runner) newPCIPassthrough() (vimtypes.BaseVirtualDevice, error) {
	dev := &vimtypes.VirtualPCIPassthrough{
		VirtualDevice: vimtypes.VirtualDevice{Key: -200},
	}

	switch {
	case r.cfg.vGPUProfile != "":
		dev.Backing = &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: r.cfg.vGPUProfile}
	case r.cfg.dvxDeviceClass != "":
		dev.Backing = &vimtypes.VirtualPCIPassthroughDvxBackingInfo{DeviceClass: r.cfg.dvxDeviceClass}
	default:
		return nil, errors.New("no -vgpu-profile or -dvx-device-class supplied")
	}

	return dev, nil
}

// addSpec wraps devices in Add device-change entries.
func addSpec(devices ...vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {
	changes := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(devices))

	for _, d := range devices {
		changes = append(changes, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    d,
		})
	}

	return changes
}

// removeSpec wraps devices in Remove device-change entries.
func removeSpec(devices ...vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {
	changes := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(devices))

	for _, d := range devices {
		changes = append(changes, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
			Device:    d,
		})
	}

	return changes
}

// requestedFrom flattens the devices carried by a device-change list, which is
// the "requested" half of every requested-vs-observed record.
func requestedFrom(changes []vimtypes.BaseVirtualDeviceConfigSpec) []deviceInfo {
	infos := make([]deviceInfo, 0, len(changes))

	for _, c := range changes {
		spec := c.GetVirtualDeviceConfigSpec()
		if spec.Device == nil {
			continue
		}

		info := deviceInfoFor(spec.Device)
		info.Kind = fmt.Sprintf("%s %s", spec.Operation, info.Kind)
		infos = append(infos, info)
	}

	return infos
}

// createVM creates a VM via folder.CreateVM and registers it for cleanup. A
// create that fails yields no reference to register; if vCenter leaves an
// orphan behind, it has to be removed by hand.
func (r *runner) createVM(
	ctx context.Context,
	spec vimtypes.VirtualMachineConfigSpec) (*object.VirtualMachine, error) {

	ctx, cancel := r.withTaskTimeout(ctx)
	defer cancel()

	task, err := r.folder.CreateVM(ctx, spec, r.resourcePool, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit CreateVM for %s: %w", spec.Name, err)
	}

	info, err := task.WaitForResultEx(ctx)
	if err != nil {
		return nil, fmt.Errorf("CreateVM failed for %s: %w", spec.Name, err)
	}

	ref, ok := info.Result.(vimtypes.ManagedObjectReference)
	if !ok {
		return nil, fmt.Errorf("CreateVM for %s returned %T, not a VM reference", spec.Name, info.Result)
	}

	vm := object.NewVirtualMachine(r.client.Client, ref)
	r.created = append(r.created, vm)

	return vm, nil
}

// withTaskTimeout bounds a single vSphere task so one wedged operation cannot
// hang the whole run.
func (r *runner) withTaskTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.cfg.taskTimeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, r.cfg.taskTimeout)
}

// reconfigure applies a ConfigSpec and waits for the task.
func (r *runner) reconfigure(
	ctx context.Context,
	vm *object.VirtualMachine,
	spec vimtypes.VirtualMachineConfigSpec) error {

	ctx, cancel := r.withTaskTimeout(ctx)
	defer cancel()

	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return fmt.Errorf("failed to submit ReconfigVM_Task: %w", err)
	}

	_, err = task.WaitForResultEx(ctx)
	if err != nil {
		return fmt.Errorf("ReconfigVM_Task failed: %w", err)
	}

	return nil
}

// addDevices is the common shape of most experiments: send an Add and record
// requested against observed.
func (r *runner) addDevices(
	ctx context.Context,
	vm *object.VirtualMachine,
	name string,
	devices ...vimtypes.BaseVirtualDevice) step {

	changes := addSpec(devices...)
	s := step{Name: name, Requested: requestedFrom(changes)}

	err := r.reconfigure(ctx, vm, vimtypes.VirtualMachineConfigSpec{DeviceChange: changes})
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
	}

	r.observeInto(ctx, vm, &s)

	return s
}

// observeInto records the VM's current ethernet cards on the step, or the
// reason the observation failed.
func (r *runner) observeInto(ctx context.Context, vm *object.VirtualMachine, s *step) {
	observed, err := r.observeEthCards(ctx, vm)
	if err != nil {
		s.Notes = append(s.Notes, fmt.Sprintf("Failed to observe hardware: %v", err))

		return
	}

	s.Observed = observed
}

// powerState changes the VM's power state and waits for it to settle.
func (r *runner) powerState(
	ctx context.Context,
	vm *object.VirtualMachine,
	on bool) error {

	ctx, cancel := r.withTaskTimeout(ctx)
	defer cancel()

	var (
		task *object.Task
		err  error
	)

	if on {
		task, err = vm.PowerOn(ctx)
	} else {
		task, err = vm.PowerOff(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to submit power task: %w", err)
	}

	_, err = task.WaitForResultEx(ctx)
	if err != nil {
		return fmt.Errorf("power task failed: %w", err)
	}

	want := vimtypes.VirtualMachinePowerStatePoweredOff
	if on {
		want = vimtypes.VirtualMachinePowerStatePoweredOn
	}

	return vm.WaitForPowerState(ctx, want)
}

// cleanup destroys every VM the program created (R7).
func (r *runner) cleanup(ctx context.Context) {
	if r.cfg.keep {
		logf("Keeping %d research VM(s) as requested by -keep", len(r.created))

		for _, vm := range r.created {
			logf("  kept: %s", vm.Reference().Value)
		}

		return
	}

	for _, vm := range r.created {
		err := destroyVM(ctx, vm)
		if err != nil {
			logf("WARNING: failed to destroy %s: %v", vm.Reference().Value, err)
		}
	}
}

// destroyVM powers off a VM if needed and destroys it. Widened from a
// PoweredOn-only check to any non-PoweredOff state so a VM E16 leaves
// suspended (if it fails between its own suspend and resume steps) is also
// powered off first — this path is defensive and was not exercised by any
// run (E16 always resumed cleanly). PowerOff is best-effort here: a failure
// is swallowed and Destroy is attempted anyway, so the worst case is a
// leaked VM plus a logged warning from cleanup, not a wedged run.
func destroyVM(ctx context.Context, vm *object.VirtualMachine) error {
	state, err := vm.PowerState(ctx)
	if err == nil && state != vimtypes.VirtualMachinePowerStatePoweredOff {
		task, offErr := vm.PowerOff(ctx)
		if offErr == nil {
			_, _ = task.WaitForResultEx(ctx)
		}
	}

	task, err := vm.Destroy(ctx)
	if err != nil {
		return fmt.Errorf("failed to submit Destroy_Task: %w", err)
	}

	_, err = task.WaitForResultEx(ctx)
	if err != nil {
		return fmt.Errorf("Destroy_Task failed: %w", err)
	}

	return nil
}

// logf writes run progress to stderr, keeping stdout clean for the report.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// honouredUnits reports whether every explicitly requested unit number appears
// on the observed hardware, and describes the ones that do not.
func honouredUnits(requested, observed []deviceInfo) (bool, []string) {
	observedUnits := make([]int32, 0, len(observed))

	for _, o := range observed {
		if o.UnitNumber != nil {
			observedUnits = append(observedUnits, *o.UnitNumber)
		}
	}

	var mismatches []string

	for _, req := range requested {
		if req.UnitNumber == nil {
			continue
		}

		if !slices.Contains(observedUnits, *req.UnitNumber) {
			mismatches = append(mismatches, fmt.Sprintf(
				"Requested unit %d is absent from the observed units %v — the task succeeded "+
					"but the platform placed the device elsewhere.", *req.UnitNumber, observedUnits))
		}
	}

	return len(mismatches) == 0, mismatches
}

// judgeHonoured sets the result status from a requested-vs-observed step,
// distinguishing "honoured" from "task succeeded but silently reassigned" (R7).
func (r *result) judgeHonoured(s step) {
	if s.Err != "" {
		r.Status = statusError
		r.Reason = s.Err

		return
	}

	ok, mismatches := honouredUnits(s.Requested, s.Observed)
	if ok {
		r.Status = statusHonoured
		r.find("Every explicitly requested unit number was observed on the resulting hardware.")

		return
	}

	r.Status = statusNotHonoured

	for _, m := range mismatches {
		r.find("%s", m)
	}
}

// explicitUnits is the non-contiguous set the explicit-placement experiments
// request. Non-contiguous on purpose: a platform that ignored the request
// would assign 7, 8, 9, which is unmistakably different from 7, 10, 16.
var explicitUnits = []int32{7, 10, 16}

// newEthCards builds one operator-shaped ethernet card per unit number, using
// nil for any entry that should be auto-assigned.
func (r *runner) newEthCards(
	ctx context.Context,
	net object.NetworkReference,
	units []*int32) ([]vimtypes.BaseVirtualDevice, error) {

	devices := make([]vimtypes.BaseVirtualDevice, 0, len(units))

	for _, u := range units {
		dev, err := r.newEthCard(ctx, net, u)
		if err != nil {
			return nil, err
		}

		devices = append(devices, dev)
	}

	return devices, nil
}

// explicitEthCards builds cards at the explicitUnits slots.
func (r *runner) explicitEthCards(ctx context.Context) ([]vimtypes.BaseVirtualDevice, error) {
	units := make([]*int32, 0, len(explicitUnits))

	for _, u := range explicitUnits {
		units = append(units, ptr.To(u))
	}

	return r.newEthCards(ctx, r.network, units)
}

// runE01 answers Q1 for the folder.CreateVM branch of the create path: does an
// explicit UnitNumber on a NIC in the initial ConfigSpec survive?
func (r *runner) runE01(ctx context.Context) *result {
	res := &result{
		ID:        e01CreateVMExplicit,
		Title:     "folder.CreateVM honours explicit NIC unit numbers in the create ConfigSpec",
		Questions: []string{"Q1"},
	}

	devices, err := r.explicitEthCards(ctx)
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(devices...)

	s := step{
		Name:      fmt.Sprintf("CreateVM with NICs at units %v", explicitUnits),
		Requested: requestedFrom(spec.DeviceChange),
	}

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
		res.Steps = append(res.Steps, s)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &s)
	res.Steps = append(res.Steps, s)
	res.judgeHonoured(s)

	return res
}

// runE02 answers Q2: does the OVF content-library deploy path honour the same
// explicit unit numbers, and how do the OVF descriptor's own NICs interact
// with the ConfigSpec's Add entries? This mirrors deployOVF in
// pkg/providers/vsphere/vmlifecycle/create_contentlibrary.go: the ConfigSpec is
// marshalled to XML and carried as deploymentSpec.VmConfigSpec.
func (r *runner) runE02(ctx context.Context) *result {
	res := &result{
		ID:        e02DeployOVFExplicit,
		Title:     "OVF content-library deploy honours explicit NIC unit numbers",
		Questions: []string{"Q1", "Q2"},
	}

	if r.cfg.libraryItem == "" {
		return res.skip("no -library-item supplied; this path needs an OVF-type content library item")
	}

	item, err := r.resolveLibraryItem(ctx)
	if err != nil {
		return res.fail(err)
	}

	if item.Type != library.ItemTypeOVF {
		return res.fail(fmt.Errorf("library item %q is type %q, not %q",
			item.Name, item.Type, library.ItemTypeOVF))
	}

	baseline := step{Name: "Deploy the OVF with no ConfigSpec NIC entries (baseline)"}

	baseVM, err := r.deployOVF(ctx, item, r.vmName(res.ID, "baseline"), nil)
	if err != nil {
		baseline.Err = err.Error()
		baseline.Faults = captureFaults(err)
	} else {
		r.observeInto(ctx, baseVM, &baseline)
		baseline.Notes = append(baseline.Notes,
			"These are the NICs the OVF descriptor itself contributes, and the slots the "+
				"platform gave them with no ConfigSpec involvement.")
	}

	res.Steps = append(res.Steps, baseline)

	devices, err := r.explicitEthCards(ctx)
	if err != nil {
		return res.fail(err)
	}

	// Only the device changes: the deployment spec owns name and placement.
	configSpec := vimtypes.VirtualMachineConfigSpec{DeviceChange: addSpec(devices...)}

	s := step{
		Name:      fmt.Sprintf("Deploy the OVF with ConfigSpec NIC Adds at units %v", explicitUnits),
		Requested: requestedFrom(configSpec.DeviceChange),
	}

	vm, err := r.deployOVF(ctx, item, r.vmName(res.ID, "configspec"), &configSpec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
		s.Notes = append(s.Notes, fmt.Sprintf(
			"A collision fault here is a result, not a program failure: it means the OVF "+
				"descriptor's own NICs already occupy one of %v. That is half of Q2's "+
				"interaction question. Record the fault, then re-run just this experiment "+
				"(-only %s) with units the baseline step shows to be free.",
			explicitUnits, res.ID))
		res.Steps = append(res.Steps, s)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &s)
	s.Notes = append(s.Notes, fmt.Sprintf(
		"Compare against the baseline step: %d NIC(s) came from the OVF alone, %d are present here.",
		len(baseline.Observed), len(s.Observed)))
	res.Steps = append(res.Steps, s)
	res.judgeHonoured(s)

	if len(s.Observed) != len(baseline.Observed)+len(explicitUnits) {
		res.find("The NIC count is not baseline + ConfigSpec adds (%d != %d + %d): the OVF's own "+
			"NICs and the ConfigSpec Add entries interact rather than accumulate. Record which.",
			len(s.Observed), len(baseline.Observed), len(explicitUnits))
	}

	return res
}

// resolveLibraryItem looks the configured content library item up by ID, then
// by name.
func (r *runner) resolveLibraryItem(ctx context.Context) (*library.Item, error) {
	m := library.NewManager(r.rest)

	item, err := m.GetLibraryItem(ctx, r.cfg.libraryItem)
	if err == nil {
		return item, nil
	}

	ids, findErr := m.FindLibraryItems(ctx, library.FindItem{Name: r.cfg.libraryItem})
	if findErr != nil {
		return nil, fmt.Errorf("failed to find library item %q: %w", r.cfg.libraryItem, findErr)
	}

	if len(ids) != 1 {
		return nil, fmt.Errorf("library item %q matched %d items; pass an item ID instead",
			r.cfg.libraryItem, len(ids))
	}

	item, err = m.GetLibraryItem(ctx, ids[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get library item %q: %w", ids[0], err)
	}

	return item, nil
}

// deployOVF deploys a content library OVF item, optionally carrying a
// ConfigSpec exactly as the product's deployOVF does.
func (r *runner) deployOVF(
	ctx context.Context,
	item *library.Item,
	name string,
	configSpec *vimtypes.VirtualMachineConfigSpec) (*object.VirtualMachine, error) {

	ctx, cancel := r.withTaskTimeout(ctx)
	defer cancel()

	mgr := vcenter.NewManager(r.rest)

	target := vcenter.Target{
		ResourcePoolID: r.resourcePool.Reference().Value,
		FolderID:       r.folder.Reference().Value,
	}

	deploymentSpec := vcenter.DeploymentSpec{
		Name:               name,
		AcceptAllEULA:      true,
		DefaultDatastoreID: r.datastore.Reference().Value,
	}

	filter, err := mgr.FilterLibraryItem(ctx, item.ID, vcenter.FilterRequest{Target: target})
	if err == nil {
		for _, n := range filter.Networks {
			deploymentSpec.NetworkMappings = append(deploymentSpec.NetworkMappings,
				vcenter.NetworkMapping{Key: n, Value: r.network.Reference().Value})
		}
	}

	if configSpec != nil {
		configSpecXML, marshalErr := pkgutil.MarshalConfigSpecToXML(*configSpec)
		if marshalErr != nil {
			return nil, fmt.Errorf("failed to marshal ConfigSpec to XML: %w", marshalErr)
		}

		deploymentSpec.VmConfigSpec = &vcenter.VmConfigSpec{
			Provider: constants.ConfigSpecProviderXML,
			XML:      base64.StdEncoding.EncodeToString(configSpecXML),
		}
	}

	ref, err := mgr.DeployLibraryItem(ctx, item.ID, vcenter.Deploy{
		DeploymentSpec: deploymentSpec,
		Target:         target,
	})
	if err != nil {
		return nil, fmt.Errorf("DeployLibraryItem failed for %s: %w", name, err)
	}

	if ref == nil {
		return nil, fmt.Errorf("DeployLibraryItem for %s returned no VM reference", name)
	}

	vm := object.NewVirtualMachine(r.client.Client, *ref)
	r.created = append(r.created, vm)

	return vm, nil
}

// runE03 answers Q1 for the reconfigure branch: does a post-create Add on a
// powered-off VM honour an explicit UnitNumber? Together with E01 this is the
// single most important result in the program — the matching design in T017
// only terminates if a requested slot is the slot the device lands in.
func (r *runner) runE03(ctx context.Context) *result {
	res := &result{
		ID:        e03ReconfigureExplicit,
		Title:     "ReconfigVM_Task Add honours explicit NIC unit numbers on a powered-off VM",
		Questions: []string{"Q1"},
	}

	vm, err := r.createVM(ctx, r.baseConfigSpec(r.vmName(res.ID, "")))
	if err != nil {
		return res.fail(err)
	}

	devices, err := r.explicitEthCards(ctx)
	if err != nil {
		return res.fail(err)
	}

	s := r.addDevices(ctx, vm, fmt.Sprintf("Add NICs at units %v to a powered-off VM", explicitUnits), devices...)
	res.Steps = append(res.Steps, s)
	res.judgeHonoured(s)

	return res
}

// runE04 confirms the platform's own assignment when no unit number is
// requested: the first NIC should land at 7 and the next at the next free slot
// in the 7-16 band. These are the values the post-create backfill records into
// the VM spec, so they need confirming rather than assuming.
func (r *runner) runE04(ctx context.Context) *result {
	res := &result{
		ID:    e04ReconfigureAuto,
		Title: "NICs added with no unit number are assigned from 7 upward",
	}

	vm, err := r.createVM(ctx, r.baseConfigSpec(r.vmName(res.ID, "")))
	if err != nil {
		return res.fail(err)
	}

	first, err := r.newEthCard(ctx, r.network, nil)
	if err != nil {
		return res.fail(err)
	}

	s1 := r.addDevices(ctx, vm, "Add the first NIC with UnitNumber nil", first)
	res.Steps = append(res.Steps, s1)

	second, err := r.newEthCard(ctx, r.network, nil)
	if err != nil {
		return res.fail(err)
	}

	s2 := r.addDevices(ctx, vm, "Add a second NIC with UnitNumber nil", second)
	res.Steps = append(res.Steps, s2)

	res.Status = statusRecorded

	if len(s1.Observed) == 1 && s1.Observed[0].UnitNumber != nil {
		res.find("First NIC landed at unit %d (expected %d).", *s1.Observed[0].UnitNumber, nicUnitNumberFirst)
	}

	if len(s2.Observed) == 2 && s2.Observed[1].UnitNumber != nil {
		res.find("Second NIC landed at unit %d.", *s2.Observed[1].UnitNumber)
	}

	res.find("Every observed unit number must fall in %d-%d for the CRD range markers in T004 to be correct.",
		nicUnitNumberFirst, nicUnitNumberLast)

	return res
}

// runE05 records add and remove behaviour powered off (the case this feature
// gates on) and powered on (informational only — NIC device changes are
// powered-off-only in the product today, see plan.md I5).
func (r *runner) runE05(ctx context.Context) *result {
	res := &result{
		ID:    e05AddRemove,
		Title: "Add and remove NICs, powered off (primary) and powered on (informational)",
	}

	devices, err := r.newEthCards(ctx, r.network, []*int32{nil, nil})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(devices...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	res.Status = statusRecorded

	initial := step{Name: "Initial hardware (two auto-assigned NICs)"}
	r.observeInto(ctx, vm, &initial)
	res.Steps = append(res.Steps, initial)

	if len(initial.Observed) < 2 {
		return res.fail(errors.New("expected two NICs after create"))
	}

	res.Steps = append(res.Steps, r.removeLastNIC(ctx, vm, "Remove the second NIC while powered off"))

	added, err := r.newEthCard(ctx, r.network, nil)
	if err != nil {
		return res.fail(err)
	}

	res.Steps = append(res.Steps,
		r.addDevices(ctx, vm, "Add a NIC with UnitNumber nil while powered off", added))

	poweredOn := step{Name: "Power on for the informational hot-add/hot-remove steps"}

	err = r.powerState(ctx, vm, true)
	if err != nil {
		poweredOn.Err = err.Error()
		poweredOn.Notes = append(poweredOn.Notes,
			"Informational only: the powered-on steps do not gate anything in this change set (I5).")
		res.Steps = append(res.Steps, poweredOn)

		return res
	}

	res.Steps = append(res.Steps, poweredOn)

	hotAdd, err := r.newEthCard(ctx, r.network, nil)
	if err != nil {
		return res.fail(err)
	}

	hotAddStep := r.addDevices(ctx, vm, "Hot-add a NIC with UnitNumber nil (informational)", hotAdd)
	hotAddStep.Notes = append(hotAddStep.Notes,
		"Informational only — the product emits no NIC device changes on a powered-on VM (I5).")
	res.Steps = append(res.Steps, hotAddStep)

	res.Steps = append(res.Steps, r.removeLastNIC(ctx, vm, "Hot-remove the last NIC (informational)"))

	err = r.powerState(ctx, vm, false)
	if err != nil {
		res.find("Failed to power the VM back off: %v", err)
	}

	return res
}

// removeLastNIC removes the highest-unit ethernet card and records the result.
func (r *runner) removeLastNIC(
	ctx context.Context, vm *object.VirtualMachine, name string) step {

	s := step{Name: name}

	devices, err := r.hardware(ctx, vm)
	if err != nil {
		s.Err = err.Error()

		return s
	}

	cards := devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))
	if len(cards) == 0 {
		s.Err = "no ethernet cards to remove"

		return s
	}

	target := cards[len(cards)-1]
	changes := removeSpec(target)
	s.Requested = requestedFrom(changes)

	err = r.reconfigure(ctx, vm, vimtypes.VirtualMachineConfigSpec{DeviceChange: changes})
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
	}

	r.observeInto(ctx, vm, &s)

	return s
}

// runE06 confirms the mechanism T017's convergence path depends on: a Remove
// of the NIC at unit N and an Add of a NIC carrying UnitNumber = N are both
// accepted in the same ReconfigVM_Task, with the removes ordered first, exactly
// as ReconcileNetworkInterfaces already emits them.
//
// vcsim is not authoritative here: its duplicate-unit-number check is scoped to
// devices sharing the same ControllerKey, and operator-built ethernet cards
// leave ControllerKey unset, so the check never fires for an operator-shaped
// payload (R3 / I26). This experiment and E08 are the only real evidence.
func (r *runner) runE06(ctx context.Context) *result {
	res := &result{
		ID:    e06SameSlotReuse,
		Title: "Remove at unit N and Add at unit N are accepted in one ReconfigVM_Task",
	}

	const targetUnit = int32(9)

	devices, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(nicUnitNumberFirst), ptr.To(targetUnit)})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(devices...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	initial := step{Name: fmt.Sprintf("Initial hardware (NICs at units %d and %d)", nicUnitNumberFirst, targetUnit)}
	r.observeInto(ctx, vm, &initial)
	res.Steps = append(res.Steps, initial)

	hardware, err := r.hardware(ctx, vm)
	if err != nil {
		return res.fail(err)
	}

	existing := findCardAtUnit(hardware, targetUnit)
	if existing == nil {
		return res.fail(fmt.Errorf("no ethernet card at unit %d to replace", targetUnit))
	}

	replacement, err := r.newEthCard(ctx, r.network, ptr.To(targetUnit))
	if err != nil {
		return res.fail(err)
	}

	// Removes first, matching the order ReconcileNetworkInterfaces emits.
	changes := append(removeSpec(existing), addSpec(replacement)...)

	s := step{
		Name:      fmt.Sprintf("Remove the NIC at unit %d and add a new one at unit %d in one task", targetUnit, targetUnit),
		Requested: requestedFrom(changes),
	}

	err = r.reconfigure(ctx, vm, vimtypes.VirtualMachineConfigSpec{DeviceChange: changes})
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
	}

	r.observeInto(ctx, vm, &s)

	removed := deviceInfoFor(existing)
	s.Notes = append(s.Notes, fmt.Sprintf("Removed device: %s", removed))
	res.Steps = append(res.Steps, s)

	if s.Err != "" {
		res.Status = statusNotHonoured
		res.find("Same-slot Remove+Add in one task was REJECTED. T017's convergence path cannot " +
			"replace a device in place and the design needs revisiting.")

		return res
	}

	res.judgeHonoured(s)

	newCard := findInfoAtUnit(s.Observed, targetUnit)
	if newCard != nil {
		// Don't assert the key changed without checking: on builds where the
		// platform derives an ethernet card's key deterministically from its
		// unit number (observed on vCenter 9.2.0: key == 4000+(unit-7)), a
		// same-slot Remove+Add legitimately reproduces the same key, and a
		// key-based check alone would misreport a genuine replacement as a
		// no-op. MAC is the reliable signal here: the platform generates a
		// fresh one for the new device.
		changedKey := newCard.Key != removed.Key
		changedMAC := newCard.MACAddress != removed.MACAddress
		res.find("The device at unit %d after the task has key %d and MAC %q; the removed device "+
			"had key %d and MAC %q (key changed: %v, MAC changed: %v). A changed MAC — and, on "+
			"builds where the key is not derived from the unit number, a changed key — confirms "+
			"the slot was genuinely reused by new hardware rather than the remove being ignored.",
			targetUnit, newCard.Key, newCard.MACAddress, removed.Key, removed.MACAddress,
			changedKey, changedMAC)

		if !changedKey {
			res.find("Key was UNCHANGED across the same-slot Remove+Add (both %d). This build "+
				"appears to derive an ethernet card's Key deterministically from its unit number "+
				"rather than from creation order; do not rely on a changed Key alone as replacement "+
				"evidence on this platform.", newCard.Key)
		}
	}

	return res
}

// findCardAtUnit returns the ethernet card occupying the given unit number.
func findCardAtUnit(devices object.VirtualDeviceList, unit int32) vimtypes.BaseVirtualDevice {
	for _, d := range devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil)) {
		u := d.GetVirtualDevice().UnitNumber
		if u != nil && *u == unit {
			return d
		}
	}

	return nil
}

// findInfoAtUnit returns the recorded device at the given unit number.
func findInfoAtUnit(infos []deviceInfo, unit int32) *deviceInfo {
	for i := range infos {
		if infos[i].UnitNumber != nil && *infos[i].UnitNumber == unit {
			return &infos[i]
		}
	}

	return nil
}

// runE07 records whether an Edit can relocate an existing NIC's unit number.
// Informational: this design never issues such an Edit, because unitNumber is
// an identity rather than a relocatable slot. The answer is context for the
// deferred Edit-instead-of-replace follow-on.
func (r *runner) runE07(ctx context.Context) *result {
	res := &result{
		ID:    e07EditUnitNumber,
		Title: "Edit an existing NIC's unit number on a powered-off VM (informational)",
	}

	const editTo = int32(11)

	devices, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(nicUnitNumberFirst)})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(devices...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	hardware, err := r.hardware(ctx, vm)
	if err != nil {
		return res.fail(err)
	}

	existing := findCardAtUnit(hardware, nicUnitNumberFirst)
	if existing == nil {
		return res.fail(fmt.Errorf("no ethernet card at unit %d to edit", nicUnitNumberFirst))
	}

	existing.GetVirtualDevice().UnitNumber = ptr.To(editTo)

	changes := []vimtypes.BaseVirtualDeviceConfigSpec{
		&vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
			Device:    existing,
		},
	}

	s := step{
		Name:      fmt.Sprintf("Edit the NIC at unit %d to unit %d", nicUnitNumberFirst, editTo),
		Requested: requestedFrom(changes),
	}

	err = r.reconfigure(ctx, vm, vimtypes.VirtualMachineConfigSpec{DeviceChange: changes})
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
	}

	r.observeInto(ctx, vm, &s)
	s.Notes = append(s.Notes,
		"Informational only: nothing in this change set issues an Edit that relocates a unit number.")
	res.Steps = append(res.Steps, s)

	res.Status = statusRecorded

	switch {
	case s.Err != "":
		res.find("The Edit was rejected. Relocating a NIC's unit number is not an available operation.")
	case findInfoAtUnit(s.Observed, editTo) != nil:
		res.find("The Edit was accepted and the NIC now occupies unit %d.", editTo)
	default:
		res.find("The Edit task succeeded but the NIC did not move to unit %d — accepted and "+
			"silently ignored, which is the worst of the three answers for a future Edit-based design.", editTo)
	}

	return res
}

// runE08 answers Q4: the exact fault vSphere returns when an explicit unit
// number collides with an existing NIC. The answer drives whether the
// reconciler treats the failure as permanent (pkgerr.NoRequeueError) or
// retries, in T018 and T029. Both reachable collisions are exercised: one
// inside a single create ConfigSpec, one against already-present hardware.
func (r *runner) runE08(ctx context.Context) *result {
	res := &result{
		ID:        e08Collision,
		Title:     "Fault returned when an explicit NIC unit number collides",
		Questions: []string{"Q4"},
	}

	res.Status = statusRecorded

	res.Steps = append(res.Steps, r.collisionAtCreate(ctx, res.ID))
	res.Steps = append(res.Steps, r.collisionAtReconfigure(ctx, res.ID))

	for _, s := range res.Steps {
		if s.Err == "" {
			res.find("Step %q did NOT fault. vSphere accepted a duplicate unit number; the "+
				"webhook uniqueness check is the only thing preventing it and the reconciler "+
				"has no fault to key error handling off.", s.Name)

			continue
		}

		for _, f := range s.Faults {
			res.find("Step %q returned fault `%s`%s.", s.Name, f.Type, deviceIndexSuffix(f))
		}
	}

	return res
}

// deviceIndexSuffix renders the deviceIndex clause of a fault, when present.
func deviceIndexSuffix(f faultInfo) string {
	if f.DeviceIndex == nil {
		return ""
	}

	return fmt.Sprintf(" with deviceIndex %d", *f.DeviceIndex)
}

// collisionAtCreate sends a create ConfigSpec carrying two NICs at the same
// unit number.
func (r *runner) collisionAtCreate(ctx context.Context, experiment string) step {
	s := step{Name: "CreateVM with two NICs both at the same unit number"}

	devices, err := r.newEthCards(ctx, r.network,
		[]*int32{ptr.To(nicUnitNumberFirst), ptr.To(nicUnitNumberFirst)})
	if err != nil {
		s.Err = err.Error()

		return s
	}

	spec := r.baseConfigSpec(r.vmName(experiment, "create-collision"))
	spec.DeviceChange = addSpec(devices...)
	s.Requested = requestedFrom(spec.DeviceChange)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)

		return s
	}

	r.observeInto(ctx, vm, &s)

	return s
}

// collisionAtReconfigure adds a NIC at a unit number an existing NIC already
// occupies. This is the collision the product can actually hit in the field:
// against a VM class ConfigSpec device, or a NIC added out of band.
func (r *runner) collisionAtReconfigure(ctx context.Context, experiment string) step {
	s := step{Name: "ReconfigVM_Task Add at a unit number an existing NIC already occupies"}

	devices, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(nicUnitNumberFirst)})
	if err != nil {
		s.Err = err.Error()

		return s
	}

	spec := r.baseConfigSpec(r.vmName(experiment, "add-collision"))
	spec.DeviceChange = addSpec(devices...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)

		return s
	}

	collide, err := r.newEthCard(ctx, r.network, ptr.To(nicUnitNumberFirst))
	if err != nil {
		s.Err = err.Error()

		return s
	}

	return r.addDevices(ctx, vm, s.Name, collide)
}

// runE09 records the ControllerKey the operator's own Add payloads carry.
// govmomi's CreateEthernetCard does not set one, so the payload leaves it
// unset and vSphere is expected to resolve the PCI controller itself. The same
// gap is why vcsim's duplicate check never fires for an operator-shaped Add
// (R3 / I26), so this is worth confirming once in practice.
func (r *runner) runE09(ctx context.Context) *result {
	res := &result{
		ID:    e09ControllerKey,
		Title: "ControllerKey on operator-built Add payloads is unset and resolved by vSphere",
	}

	card, err := r.newEthCard(ctx, r.network, nil)
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(card)

	s := step{Name: "CreateVM with an operator-shaped NIC (ControllerKey left unset)",
		Requested: requestedFrom(spec.DeviceChange)}

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
		res.Steps = append(res.Steps, s)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &s)

	hardware, err := r.hardware(ctx, vm)
	if err != nil {
		return res.fail(err)
	}

	controllers := hardware.SelectByType((*vimtypes.VirtualPCIController)(nil))
	for _, c := range controllers {
		s.Notes = append(s.Notes, fmt.Sprintf("PCI controller: %s", deviceInfoFor(c)))
	}

	res.Steps = append(res.Steps, s)
	res.Status = statusRecorded

	if len(s.Requested) > 0 {
		res.find("The requested payload carried ControllerKey=%d (unset).", s.Requested[0].ControllerKey)
	}

	if len(s.Observed) > 0 && len(controllers) > 0 {
		pciKey := controllers[0].GetVirtualDevice().Key
		observedKey := s.Observed[0].ControllerKey

		if observedKey == pciKey {
			res.find("vSphere resolved the NIC onto the PCI controller (key %d) with no operator "+
				"involvement. No design change follows.", pciKey)
		} else {
			res.find("The NIC's observed ControllerKey %d is not the PCI controller key %d — "+
				"investigate before relying on implicit controller resolution.", observedKey, pciKey)
		}
	}

	return res
}

// runE10 confirms that NICs still land in 7-16 on a VM whose virtual PCI bus
// also hosts a non-NIC occupant, and that the passthrough device takes its own
// band. The platform allocates PCI units statically per device class
// (external/vim/api/v1alpha1/testdata/device_keys.txt), so this confirms the
// documented contract rather than discovering the range. Nothing is gated on
// it, but the range bound in T004 and T011 rests on it.
func (r *runner) runE10(ctx context.Context) *result {
	res := &result{
		ID:    e10NonNICPCIOccupant,
		Title: "NICs keep the 7-16 band on a PCI bus shared with a passthrough device",
	}

	passthrough, err := r.newPCIPassthrough()
	if err != nil {
		return res.skip("no -vgpu-profile or -dvx-device-class supplied; this experiment needs a " +
			"passthrough-capable host and a device to attach")
	}

	cards, err := r.newEthCards(ctx, r.network, []*int32{nil, nil})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(append(cards, passthrough)...)

	s := step{
		Name:      "CreateVM with two auto-assigned NICs and one PCI passthrough device",
		Requested: requestedFrom(spec.DeviceChange),
	}

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)
		res.Steps = append(res.Steps, s)

		return res.fail(err)
	}

	onBus, err := r.observePCIBus(ctx, vm)
	if err != nil {
		return res.fail(err)
	}

	s.Observed = onBus
	s.Notes = append(s.Notes, "Observed devices are the whole virtual PCI bus, not only the NICs.")
	res.Steps = append(res.Steps, s)
	res.Status = statusRecorded

	for _, d := range onBus {
		if d.UnitNumber == nil {
			continue
		}

		inNICBand := *d.UnitNumber >= nicUnitNumberFirst && *d.UnitNumber <= nicUnitNumberLast
		isNIC := strings.Contains(strings.ToLower(d.Kind), "ethernet") ||
			strings.Contains(strings.ToLower(d.Kind), "vmxnet") ||
			strings.Contains(strings.ToLower(d.Kind), "e1000")

		if isNIC && !inNICBand {
			res.find("NIC %s fell OUTSIDE the %d-%d band — the CRD range markers in T004 are wrong.",
				d.Kind, nicUnitNumberFirst, nicUnitNumberLast)
		}

		if !isNIC && inNICBand {
			res.find("Non-NIC device %s occupies unit %d, inside the NIC band %d-%d — the band is "+
				"not NIC-exclusive and the uniqueness model needs revisiting.",
				d.Kind, *d.UnitNumber, nicUnitNumberFirst, nicUnitNumberLast)
		}
	}

	if len(res.Findings) == 0 {
		res.find("Every NIC stayed within %d-%d and no non-NIC device entered the band.",
			nicUnitNumberFirst, nicUnitNumberLast)
	}

	return res
}

// runE11 answers Q5: after a NIC is removed, does a subsequent auto-assigned
// Add reuse the freed slot or take the next available one?
func (r *runner) runE11(ctx context.Context) *result {
	res := &result{
		ID:        e11FreedSlotReuse,
		Title:     "Does an auto-assigned NIC reuse a freed unit number?",
		Questions: []string{"Q5"},
	}

	cards, err := r.newEthCards(ctx, r.network, []*int32{nil, nil, nil})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(cards...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	initial := step{Name: "Initial hardware (three auto-assigned NICs)"}
	r.observeInto(ctx, vm, &initial)
	res.Steps = append(res.Steps, initial)

	if len(initial.Observed) < 3 {
		return res.fail(errors.New("expected three NICs after create"))
	}

	middle := initial.Observed[1]
	if middle.UnitNumber == nil {
		return res.fail(errors.New("the middle NIC has no unit number"))
	}

	freed := *middle.UnitNumber
	highest := initial.Observed[len(initial.Observed)-1]

	hardware, err := r.hardware(ctx, vm)
	if err != nil {
		return res.fail(err)
	}

	target := findCardAtUnit(hardware, freed)
	if target == nil {
		return res.fail(fmt.Errorf("no ethernet card at unit %d", freed))
	}

	removeStep := step{Name: fmt.Sprintf("Remove the middle NIC at unit %d", freed)}
	changes := removeSpec(target)
	removeStep.Requested = requestedFrom(changes)

	err = r.reconfigure(ctx, vm, vimtypes.VirtualMachineConfigSpec{DeviceChange: changes})
	if err != nil {
		removeStep.Err = err.Error()
		removeStep.Faults = captureFaults(err)
	}

	r.observeInto(ctx, vm, &removeStep)
	res.Steps = append(res.Steps, removeStep)

	added, err := r.newEthCard(ctx, r.network, nil)
	if err != nil {
		return res.fail(err)
	}

	addStep := r.addDevices(ctx, vm, "Add a NIC with UnitNumber nil", added)
	res.Steps = append(res.Steps, addStep)
	res.Status = statusRecorded

	newUnits := unitsOf(addStep.Observed)

	switch {
	case slices.Contains(newUnits, freed):
		res.find("vSphere REUSED the freed unit %d for the new NIC.", freed)
	case highest.UnitNumber != nil && slices.Contains(newUnits, *highest.UnitNumber+1):
		res.find("vSphere did NOT reuse unit %d; the new NIC took the next available unit %d.",
			freed, *highest.UnitNumber+1)
	default:
		res.find("The new NIC landed among units %v; unit %d was freed. Record which slot it took.",
			newUnits, freed)
	}

	return res
}

// unitsOf extracts the assigned unit numbers from a device record list.
func unitsOf(infos []deviceInfo) []int32 {
	units := make([]int32, 0, len(infos))

	for _, d := range infos {
		if d.UnitNumber != nil {
			units = append(units, *d.UnitNumber)
		}
	}

	return units
}

// runE12 answers Q6: are NIC unit numbers stable across a power cycle?
func (r *runner) runE12(ctx context.Context) *result {
	res := &result{
		ID:        e12PowerCycleStability,
		Title:     "NIC unit numbers are stable across a power cycle",
		Questions: []string{"Q6"},
	}

	cards, err := r.newEthCards(ctx, r.network,
		[]*int32{ptr.To(nicUnitNumberFirst), ptr.To(int32(9)), ptr.To(int32(12))})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(cards...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	before := step{Name: "Units before the power cycle"}
	r.observeInto(ctx, vm, &before)
	res.Steps = append(res.Steps, before)

	onStep := step{Name: "Units while powered on"}

	err = r.powerState(ctx, vm, true)
	if err != nil {
		onStep.Err = err.Error()
		res.Steps = append(res.Steps, onStep)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &onStep)
	res.Steps = append(res.Steps, onStep)

	afterStep := step{Name: "Units after powering back off"}

	err = r.powerState(ctx, vm, false)
	if err != nil {
		afterStep.Err = err.Error()
		res.Steps = append(res.Steps, afterStep)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &afterStep)
	res.Steps = append(res.Steps, afterStep)
	res.Status = statusRecorded

	beforeUnits := unitsOf(before.Observed)
	onUnits := unitsOf(onStep.Observed)
	afterUnits := unitsOf(afterStep.Observed)

	if slices.Equal(beforeUnits, onUnits) && slices.Equal(beforeUnits, afterUnits) {
		res.find("Unit numbers were unchanged across the power cycle: %v.", beforeUnits)

		return res
	}

	res.find("Unit numbers CHANGED across the power cycle: before %v, powered on %v, after %v. "+
		"A unit number is not a stable identity and the matching design in T017 must be revisited.",
		beforeUnits, onUnits, afterUnits)

	return res
}

// runE13 answers Q7: does an out-of-band NIC add through the vCenter UI shift
// the unit numbers of existing NICs? No design change follows from the answer
// (I11); it is recorded so operators and the docs in T030 can state what to
// expect. The add has to be made by hand, so the experiment pauses for it.
func (r *runner) runE13(ctx context.Context) *result {
	res := &result{
		ID:        e13OutOfBandAdd,
		Title:     "Out-of-band NIC add through the vCenter UI",
		Questions: []string{"Q7"},
	}

	if !r.cfg.interactive {
		return res.skip("requires a manual vCenter UI step; re-run with -interactive to perform it")
	}

	cards, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(nicUnitNumberFirst), ptr.To(int32(9))})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(cards...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	before := step{Name: fmt.Sprintf("Units before the out-of-band add (NICs at %d and 9)", nicUnitNumberFirst)}
	r.observeInto(ctx, vm, &before)
	res.Steps = append(res.Steps, before)

	logf("")
	logf("=== %s needs a manual step ===", res.ID)
	logf("In the vCenter UI, add a network adapter to VM %q (%s),",
		spec.Name, vm.Reference().Value)
	logf("then press Enter here to re-inspect the hardware.")
	waitForEnter()

	after := step{Name: "Units after the out-of-band add"}
	r.observeInto(ctx, vm, &after)
	res.Steps = append(res.Steps, after)
	res.Status = statusRecorded

	beforeUnits := unitsOf(before.Observed)
	afterUnits := unitsOf(after.Observed)

	if len(afterUnits) == len(beforeUnits) {
		return res.fail(errors.New("no NIC appears to have been added; the manual step did not happen"))
	}

	preserved := true

	for _, u := range beforeUnits {
		if !slices.Contains(afterUnits, u) {
			preserved = false
		}
	}

	if preserved {
		res.find("The existing NICs kept units %v; the out-of-band NIC took the remainder of %v.",
			beforeUnits, afterUnits)

		return res
	}

	res.find("An out-of-band add SHIFTED existing NICs: before %v, after %v. Operators must expect "+
		"this and the docs in T030 should say so.", beforeUnits, afterUnits)

	return res
}

// waitForEnter blocks until the operator presses Enter.
func waitForEnter() {
	var line string

	_, _ = fmt.Scanln(&line)
}

// runE14 answers Q3: does a VirtualSriovEthernetCard draw from the same 7-16
// unit-number space as other NIC types? Expected, since it is a
// VirtualEthernetCard, but the spec asserts SR-IOV is covered by the same
// placement, validation, backfill, and matching, so confirm it.
//
// The experiment needs an SR-IOV-capable pNIC and network, which not every
// testbed has (R5). An unavailable environment is recorded as an explicit skip.
func (r *runner) runE14(ctx context.Context) *result {
	res := &result{
		ID:        e14SRIOV,
		Title:     "SR-IOV ethernet cards share the 7-16 unit-number space",
		Questions: []string{"Q3"},
	}

	if r.cfg.sriovNetwork == "" {
		return res.skip("no -sriov-network supplied; this experiment needs an SR-IOV-capable " +
			"pNIC/host and a suitable network (R5)")
	}

	sriovNet, err := r.finder.Network(ctx, r.cfg.sriovNetwork)
	if err != nil {
		return res.fail(fmt.Errorf("failed to find SR-IOV network %q: %w", r.cfg.sriovNetwork, err))
	}

	card, err := r.newEthCard(ctx, r.network, ptr.To(nicUnitNumberFirst))
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(card)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	sriovAuto, err := r.newSriovCard(ctx, sriovNet, nil)
	if err != nil {
		return res.fail(err)
	}

	autoStep := r.addDevices(ctx, vm, "Add an SR-IOV card with UnitNumber nil", sriovAuto)
	res.Steps = append(res.Steps, autoStep)

	sriovExplicit, err := r.newSriovCard(ctx, sriovNet, ptr.To(int32(13)))
	if err != nil {
		return res.fail(err)
	}

	explicitStep := r.addDevices(ctx, vm, "Add an SR-IOV card at explicit unit 13", sriovExplicit)
	res.Steps = append(res.Steps, explicitStep)
	res.Status = statusRecorded

	for _, d := range explicitStep.Observed {
		if !strings.Contains(d.Kind, "Sriov") || d.UnitNumber == nil {
			continue
		}

		if *d.UnitNumber >= nicUnitNumberFirst && *d.UnitNumber <= nicUnitNumberLast {
			res.find("SR-IOV card %s occupies unit %d, inside the %d-%d NIC band.",
				d.Kind, *d.UnitNumber, nicUnitNumberFirst, nicUnitNumberLast)
		} else {
			res.find("SR-IOV card %s occupies unit %d, OUTSIDE the %d-%d NIC band — the spec's "+
				"claim that SR-IOV shares the NIC unit-number space is wrong.",
				d.Kind, *d.UnitNumber, nicUnitNumberFirst, nicUnitNumberLast)
		}
	}

	return res
}

// runE15 records whether a hot-add to a powered-on VM honours an explicit unit
// number. Informational only: NIC device changes are powered-off-only in the
// product today, so nothing in this change set consumes the answer (I5). It is
// context for the tracked powered-on convergence follow-on, T032.
func (r *runner) runE15(ctx context.Context) *result {
	res := &result{
		ID:    e15HotAddExplicit,
		Title: "Hot-add a NIC with an explicit unit number (informational)",
	}

	card, err := r.newEthCard(ctx, r.network, ptr.To(nicUnitNumberFirst))
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(card)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	err = r.powerState(ctx, vm, true)
	if err != nil {
		return res.fail(err)
	}

	hotAdd, err := r.newEthCard(ctx, r.network, ptr.To(int32(12)))
	if err != nil {
		return res.fail(err)
	}

	s := r.addDevices(ctx, vm, "Hot-add a NIC at explicit unit 12", hotAdd)
	s.Notes = append(s.Notes,
		"Informational only — this change set never emits a NIC device change on a powered-on VM (I5).")
	res.Steps = append(res.Steps, s)

	offErr := r.powerState(ctx, vm, false)
	if offErr != nil {
		res.find("Failed to power the VM back off: %v", offErr)
	}

	res.judgeHonoured(s)

	return res
}

// runE16 confirms whether NIC unit numbers survive a suspend/resume cycle.
// Not one of tasks.md T001's enumerated items, and not covered by any of
// spec.md's Q1-Q8 — added to characterise a lifecycle transition the
// original scope omitted. Like every other experiment in this program, it
// observes VirtualDevice.UnitNumber from config.hardware.device — the
// persisted VM hardware configuration, which is the field's only
// representation in the vSphere API (see the file header). It therefore
// answers "does suspend/resume rewrite the config" but says nothing about
// guest-visible NIC enumeration, which this program does not check anywhere.
func (r *runner) runE16(ctx context.Context) *result {
	res := &result{
		ID:    e16SuspendResume,
		Title: "NIC unit numbers are stable across a suspend/resume cycle",
	}

	cards, err := r.newEthCards(ctx, r.network,
		[]*int32{ptr.To(nicUnitNumberFirst), ptr.To(int32(9)), ptr.To(int32(12))})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(cards...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	if err := r.powerState(ctx, vm, true); err != nil {
		return res.fail(err)
	}

	before := step{Name: "Units powered on, before suspend"}
	r.observeInto(ctx, vm, &before)
	res.Steps = append(res.Steps, before)

	suspended := step{Name: "Units while suspended"}

	if err := r.suspend(ctx, vm); err != nil {
		suspended.Err = err.Error()
		suspended.Faults = captureFaults(err)
		res.Steps = append(res.Steps, suspended)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &suspended)
	res.Steps = append(res.Steps, suspended)

	resumed := step{Name: "Units after resume"}

	// Resuming a suspended VM is the same PowerOnVM_Task as a cold power-on.
	if err := r.powerState(ctx, vm, true); err != nil {
		resumed.Err = err.Error()
		res.Steps = append(res.Steps, resumed)

		return res.fail(err)
	}

	r.observeInto(ctx, vm, &resumed)
	res.Steps = append(res.Steps, resumed)
	res.Status = statusRecorded

	beforeUnits := unitsOf(before.Observed)
	suspendedUnits := unitsOf(suspended.Observed)
	resumedUnits := unitsOf(resumed.Observed)

	if slices.Equal(beforeUnits, suspendedUnits) && slices.Equal(beforeUnits, resumedUnits) {
		res.find("Unit numbers were unchanged across suspend/resume: %v.", beforeUnits)

		return res
	}

	res.find("Unit numbers CHANGED across suspend/resume: before %v, suspended %v, resumed %v. "+
		"A unit number is not a stable identity across suspend/resume on this build and the "+
		"matching design must account for it.", beforeUnits, suspendedUnits, resumedUnits)

	return res
}

// runE17 confirms whether NIC unit numbers survive a vMotion — a host change
// via MigrateVM_Task with no storage move — to a different host in the same
// cluster as the VM's current host. Not one of tasks.md T001's enumerated
// items, and not covered by any of spec.md's Q1-Q8 — added to characterise
// a lifecycle transition the original scope omitted. Same scope caveat as
// runE16: this observes VirtualDevice.UnitNumber from
// config.hardware.device, so it answers "does vMotion rewrite the config",
// not anything about guest-visible NIC enumeration. The host change itself
// is independently confirmed via vm.HostSystem, so the migration genuinely
// happened; only the "unit number" half of the claim is config-level.
func (r *runner) runE17(ctx context.Context) *result {
	res := &result{
		ID:    e17VMotion,
		Title: "NIC unit numbers are stable across vMotion to another host",
	}

	hosts, err := r.clusterHosts(ctx)
	if err != nil {
		return res.fail(err)
	}

	if len(hosts) < 2 {
		return res.skip("fewer than two ESX hosts in the resource pool's cluster; vMotion needs " +
			"a second host to migrate to")
	}

	cards, err := r.newEthCards(ctx, r.network,
		[]*int32{ptr.To(nicUnitNumberFirst), ptr.To(int32(9)), ptr.To(int32(12))})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.DeviceChange = addSpec(cards...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	if err := r.powerState(ctx, vm, true); err != nil {
		return res.fail(err)
	}

	before := step{Name: "Units before vMotion"}
	r.observeInto(ctx, vm, &before)
	res.Steps = append(res.Steps, before)

	currentHost, err := vm.HostSystem(ctx)
	if err != nil {
		return res.fail(fmt.Errorf("failed to determine current host: %w", err))
	}

	var target *object.HostSystem

	for _, h := range hosts {
		if h.Reference().Value != currentHost.Reference().Value {
			target = h

			break
		}
	}

	if target == nil {
		return res.skip("no host distinct from the VM's current host was found in its cluster")
	}

	currentName := r.hostName(ctx, currentHost)
	targetName := r.hostName(ctx, target)

	migrateStep := step{Name: fmt.Sprintf("vMotion from %s to %s", currentName, targetName)}

	migrateCtx, cancel := r.withTaskTimeout(ctx)

	task, err := vm.Migrate(migrateCtx, nil, target, vimtypes.VirtualMachineMovePriorityDefaultPriority, "")
	if err == nil {
		_, err = task.WaitForResultEx(migrateCtx)
	}

	cancel()

	if err != nil {
		migrateStep.Err = fmt.Errorf("MigrateVM_Task failed: %w", err).Error()
		migrateStep.Faults = captureFaults(err)
		res.Steps = append(res.Steps, migrateStep)

		return res.fail(err)
	}

	res.Steps = append(res.Steps, migrateStep)

	after := step{Name: "Units after vMotion"}
	r.observeInto(ctx, vm, &after)
	res.Steps = append(res.Steps, after)
	res.Status = statusRecorded

	if newHost, hostErr := vm.HostSystem(ctx); hostErr == nil {
		res.find("VM ran on host %q before, %q after.", currentName, r.hostName(ctx, newHost))
	}

	beforeUnits := unitsOf(before.Observed)
	afterUnits := unitsOf(after.Observed)

	if slices.Equal(beforeUnits, afterUnits) {
		res.find("Unit numbers were unchanged across vMotion: %v.", beforeUnits)

		return res
	}

	res.find("Unit numbers CHANGED across vMotion: before %v, after %v. A unit number is not a "+
		"stable identity across vMotion on this build and the matching design must account "+
		"for it.", beforeUnits, afterUnits)

	return res
}

// e18HardwareVersionUpgradeDefault is the default for the -e18-start-version
// flag. It is deliberately older than this environment's observed
// create-time default (vmx-23, per E01) so UpgradeVM_Task has somewhere to
// go. vmx-15 is already known-good here: it's the version the photon-5.0
// OVF descriptor itself declares (E02's baseline step). A different
// environment whose host floor is already at or above vmx-15 should pass
// -e18-start-version explicitly rather than edit this constant.
const e18HardwareVersionUpgradeDefault = "vmx-15"

// runE18 confirms whether NIC unit numbers survive one or more
// hardware-version (VM Compatibility) upgrades via UpgradeVM_Task, walking
// through -e18-target-versions in order (e.g. an older intermediate version
// like vmx-17 or vmx-20, not only the host's current maximum), and whether
// powering the VM on at each new version disturbs them. Not one of
// tasks.md T001's enumerated items, and not covered by any of spec.md's
// Q1-Q8 — added to characterise lifecycle transitions the original scope
// omitted. Same config-vs-guest-visible scope caveat as runE16/runE17: this
// observes VirtualDevice.UnitNumber from config.hardware.device, not
// anything a guest OS would report — a power-on here confirms the platform
// re-derives the same config on boot, not that a guest OS enumerates the
// NIC identically.
func (r *runner) runE18(ctx context.Context) *result {
	res := &result{
		ID: e18HardwareVersionUpgrade,
		Title: "NIC unit numbers are stable across hardware-version (VM Compatibility) upgrades, " +
			"including after powering on at each new version",
	}

	targets := splitVersionList(r.cfg.e18TargetVersions)
	if len(targets) == 0 {
		return res.skip("no -e18-target-versions were configured")
	}

	cards, err := r.newEthCards(ctx, r.network,
		[]*int32{ptr.To(nicUnitNumberFirst), ptr.To(int32(9)), ptr.To(int32(12))})
	if err != nil {
		return res.fail(err)
	}

	spec := r.baseConfigSpec(r.vmName(res.ID, ""))
	spec.Version = r.cfg.e18StartVersion
	spec.DeviceChange = addSpec(cards...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		return res.fail(err)
	}

	startVersion, err := r.vmHardwareVersion(ctx, vm)
	if err != nil {
		return res.fail(err)
	}

	initial := step{Name: fmt.Sprintf("Units at hardware version %s, before any upgrade", startVersion)}
	r.observeInto(ctx, vm, &initial)
	res.Steps = append(res.Steps, initial)

	baselineUnits := unitsOf(initial.Observed)
	finalUnits := baselineUnits
	priorVersion := startVersion
	priorUnits := baselineUnits
	ranAnyUpgrade := false

	for i, target := range targets {
		requestedVersion := target
		if strings.EqualFold(target, "max") {
			// An empty version string means "upgrade to the most current
			// virtual hardware the host supports" — the ordinary "upgrade
			// compatibility" UI action, not a request for a specific
			// version. "max" is this program's readable stand-in for it.
			requestedVersion = ""
		}

		upgradeStep := step{Name: fmt.Sprintf(
			"UpgradeVM_Task from %s toward %s", priorVersion, displayVersion(target))}

		upgradeCtx, cancel := r.withTaskTimeout(ctx)

		task, err := vm.UpgradeVM(upgradeCtx, requestedVersion)
		if err == nil {
			_, err = task.WaitForResultEx(upgradeCtx)
		}

		cancel()

		if err != nil {
			faults := captureFaults(err)
			alreadyUpgraded := false

			for _, f := range faults {
				// Match by prefix, not equality: govmomi has both an
				// AlreadyUpgraded fault-info type and a distinct
				// AlreadyUpgradedFault wrapper type, and it was never
				// observed which spelling a real vCenter sends back over
				// this path since the fault never fired against this
				// environment's targets.
				if strings.HasPrefix(f.Type, "AlreadyUpgraded") {
					alreadyUpgraded = true

					break
				}
			}

			if alreadyUpgraded {
				upgradeStep.Notes = append(upgradeStep.Notes, fmt.Sprintf(
					"VM was already at or beyond %s; skipping to the next target",
					displayVersion(target)))
				res.Steps = append(res.Steps, upgradeStep)

				continue
			}

			upgradeStep.Err = fmt.Errorf("UpgradeVM_Task failed: %w", err).Error()
			upgradeStep.Faults = faults
			res.Steps = append(res.Steps, upgradeStep)

			if !ranAnyUpgrade {
				return res.fail(err)
			}

			// A later target in the list failed after at least one earlier
			// upgrade in this same run already succeeded; keep the results
			// already gathered instead of discarding them.
			res.find("UpgradeVM_Task toward %s failed after %d earlier upgrade step(s) in this "+
				"run succeeded: %v", displayVersion(target), i, err)

			break
		}

		ranAnyUpgrade = true
		res.Steps = append(res.Steps, upgradeStep)

		afterUpgradeVersion, err := r.vmHardwareVersion(ctx, vm)
		if err != nil {
			return res.fail(err)
		}

		afterUpgrade := step{Name: fmt.Sprintf(
			"Units at hardware version %s, powered off, right after upgrade", afterUpgradeVersion)}
		r.observeInto(ctx, vm, &afterUpgrade)
		res.Steps = append(res.Steps, afterUpgrade)

		res.find("Hardware version was %q before this upgrade step, %q after.",
			priorVersion, afterUpgradeVersion)

		if priorVersion == afterUpgradeVersion {
			res.find("UpgradeVM_Task toward %s reported success but the version string did not "+
				"change; treat the unit-number result for this step cautiously since no real "+
				"upgrade may have occurred.", displayVersion(target))
		}

		upgradeUnits := unitsOf(afterUpgrade.Observed)

		if slices.Equal(priorUnits, upgradeUnits) {
			res.find("Unit numbers were unchanged by the upgrade from %s to %q (still powered "+
				"off): %v.", priorVersion, afterUpgradeVersion, upgradeUnits)
		} else {
			res.find("Unit numbers CHANGED by the upgrade from %s to %q, while still powered "+
				"off: before %v, after %v. This upgrade step alone is not unit-number-safe on "+
				"this build.", priorVersion, afterUpgradeVersion, priorUnits, upgradeUnits)
		}

		if err := r.powerState(ctx, vm, true); err != nil {
			return res.fail(fmt.Errorf(
				"failed to power on VM after upgrading to %s: %w", afterUpgradeVersion, err))
		}

		poweredOn := step{Name: fmt.Sprintf(
			"Units at hardware version %s, powered ON after upgrade", afterUpgradeVersion)}
		r.observeInto(ctx, vm, &poweredOn)
		res.Steps = append(res.Steps, poweredOn)

		poweredOnUnits := unitsOf(poweredOn.Observed)

		if slices.Equal(upgradeUnits, poweredOnUnits) {
			res.find("Unit numbers were unchanged by powering on at hardware version %q: %v.",
				afterUpgradeVersion, poweredOnUnits)
		} else {
			res.find("Unit numbers CHANGED when the VM was powered on after the upgrade to %q: "+
				"powered-off %v, powered-on %v. Powering on a freshly-upgraded VM is not "+
				"unit-number-safe on this build.", afterUpgradeVersion, upgradeUnits, poweredOnUnits)
		}

		finalUnits = poweredOnUnits

		// Power back off so the next target's UpgradeVM_Task (which requires
		// a powered-off VM, same as the first one) has a clean precondition.
		if err := r.powerState(ctx, vm, false); err != nil {
			return res.fail(fmt.Errorf(
				"failed to power off VM after observing it powered on at %s: %w",
				afterUpgradeVersion, err))
		}

		priorVersion = afterUpgradeVersion
		priorUnits = poweredOnUnits
	}

	if !ranAnyUpgrade {
		return res.skip(fmt.Sprintf(
			"VM was already at or beyond every requested target version (%s); no upgrade was "+
				"available to test", r.cfg.e18TargetVersions))
	}

	res.Status = statusRecorded

	if slices.Equal(baselineUnits, finalUnits) {
		res.find("Unit numbers were unchanged end-to-end, from %s through every upgrade and "+
			"power-cycle step above: %v.", startVersion, baselineUnits)

		return res
	}

	res.find("Unit numbers CHANGED end-to-end: started at %s with %v, ended with %v after all "+
		"upgrade and power-cycle steps above. A unit number is not a stable identity across a "+
		"VM Compatibility upgrade on this build and the matching/backfill design must account "+
		"for it.", startVersion, baselineUnits, finalUnits)

	return res
}

// runE19 asks what vSphere does, on its own, with an explicit UnitNumber
// outside the platform's 7-16 ethernet-card band — not gated on any Q1-Q8
// question, and not something the webhook design (tasks.md T013, spec.md G9)
// depends on: the CRD/webhook is expected to reject an out-of-range value
// before it ever reaches vSphere. This experiment exists to confirm that
// vSphere itself does not silently accept or silently renumber such a value,
// which is what the backfill's defensive out-of-range skip (spec.md G7) and
// the CRD range markers assume without observing.
//
// Three distinct out-of-range values are tried, each probing a different
// static PCI-unit band (device_keys.txt): 3 (SCSI HBA band, unoccupied on
// this diskless research VM), 17 (VMCI's own reserved unit), and 200 (no
// PCI-unit meaning on this platform at all). Each is tried on both a create
// ConfigSpec and a ReconfigVM_Task Add against an already-created VM, so a
// difference between the two Add paths (as Q1/Q2 found for OVF vs.
// folder.CreateVM) would not go unnoticed.
func (r *runner) runE19(ctx context.Context) *result {
	res := &result{
		ID:    e19OutOfRangeUnitNumber,
		Title: "vSphere's handling of an explicit NIC UnitNumber outside the 7-16 band",
	}

	res.Status = statusRecorded

	for _, unit := range outOfRangeUnits {
		res.Steps = append(res.Steps, r.outOfRangeAtCreate(ctx, res.ID, unit))
		res.Steps = append(res.Steps, r.outOfRangeAtReconfigure(ctx, res.ID, unit))
	}

	for _, s := range res.Steps {
		if s.Err != "" {
			for _, f := range s.Faults {
				res.find("Step %q returned fault `%s`%s — vSphere rejects this out-of-range "+
					"value outright.", s.Name, f.Type, deviceIndexSuffix(f))
			}

			if len(s.Faults) == 0 {
				res.find("Step %q failed with a non-fault error: %s", s.Name, s.Err)
			}

			continue
		}

		ok, mismatches := honouredUnits(s.Requested, s.Observed)
		if ok {
			res.find("Step %q succeeded and the out-of-range unit number was honoured "+
				"exactly as requested — vSphere does not reject or renumber it at this "+
				"layer; only the webhook stands between this value and hardware.", s.Name)

			continue
		}

		for _, m := range mismatches {
			res.find("Step %q succeeded but was silently reassigned: %s", s.Name, m)
		}
	}

	return res
}

// outOfRangeAtCreate sends a create ConfigSpec carrying a single NIC at an
// out-of-range unit number.
func (r *runner) outOfRangeAtCreate(ctx context.Context, experiment string, unit int32) step {
	s := step{Name: fmt.Sprintf("CreateVM with a NIC at out-of-range unit %d", unit)}

	devices, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(unit)})
	if err != nil {
		s.Err = err.Error()

		return s
	}

	spec := r.baseConfigSpec(r.vmName(experiment, fmt.Sprintf("create-oor-%d", unit)))
	spec.DeviceChange = addSpec(devices...)
	s.Requested = requestedFrom(spec.DeviceChange)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = err.Error()
		s.Faults = captureFaults(err)

		return s
	}

	r.observeInto(ctx, vm, &s)

	return s
}

// outOfRangeAtReconfigure creates a VM with one ordinary in-range NIC, then
// adds a second NIC at an out-of-range unit number via ReconfigVM_Task —
// the shape a webhook bypass or a stale spec value would actually take in
// the field, as opposed to a bare create.
func (r *runner) outOfRangeAtReconfigure(ctx context.Context, experiment string, unit int32) step {
	s := step{Name: fmt.Sprintf("ReconfigVM_Task Add at out-of-range unit %d", unit)}

	baseDevices, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(nicUnitNumberFirst)})
	if err != nil {
		s.Err = err.Error()

		return s
	}

	spec := r.baseConfigSpec(r.vmName(experiment, fmt.Sprintf("reconfig-oor-%d", unit)))
	spec.DeviceChange = addSpec(baseDevices...)

	vm, err := r.createVM(ctx, spec)
	if err != nil {
		s.Err = fmt.Sprintf("failed to create base VM: %v", err)

		return s
	}

	oorDevices, err := r.newEthCards(ctx, r.network, []*int32{ptr.To(unit)})
	if err != nil {
		s.Err = err.Error()

		return s
	}

	s = r.addDevices(ctx, vm, s.Name, oorDevices...)

	return s
}

// displayVersion renders the "max" sentinel back into something readable in
// step names and findings, instead of the empty string UpgradeVM actually
// receives for it.
func displayVersion(target string) string {
	if strings.EqualFold(target, "max") {
		return "the host's maximum supported version"
	}

	return target
}

// splitVersionList parses a comma-separated hardware-version list for
// -e18-target-versions, trimming whitespace and dropping empty entries.
// Unlike splitIDs, it does not upper-case entries — hardware version
// strings (e.g. "vmx-17") are passed to UpgradeVM verbatim. The "max"
// sentinel is matched case-insensitively by its own callers (displayVersion,
// runE18), not here.
func splitVersionList(s string) []string {
	parts := strings.Split(s, ",")
	versions := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			versions = append(versions, p)
		}
	}

	return versions
}

// vmHardwareVersion returns a VM's current hardware version (e.g. "vmx-15"),
// read fresh from vCenter. Unlike hardware(), which updates the run's
// recorded environment version as a side effect on its first call, this is
// side-effect-free — E18 needs to observe the version changing across a
// single VM's lifetime, which the once-only environment field can't express.
func (r *runner) vmHardwareVersion(ctx context.Context, vm *object.VirtualMachine) (string, error) {
	var moVM mo.VirtualMachine

	err := vm.Properties(ctx, vm.Reference(), []string{"config.version"}, &moVM)
	if err != nil {
		return "", fmt.Errorf("failed to fetch hardware version for %s: %w", vm.Reference().Value, err)
	}

	if moVM.Config == nil {
		return "", fmt.Errorf("VM %s has no config", vm.Reference().Value)
	}

	return moVM.Config.Version, nil
}

// suspend suspends a powered-on VM and waits for the task.
func (r *runner) suspend(ctx context.Context, vm *object.VirtualMachine) error {
	ctx, cancel := r.withTaskTimeout(ctx)
	defer cancel()

	task, err := vm.Suspend(ctx)
	if err != nil {
		return fmt.Errorf("failed to submit SuspendVM_Task: %w", err)
	}

	_, err = task.WaitForResultEx(ctx)
	if err != nil {
		return fmt.Errorf("SuspendVM_Task failed: %w", err)
	}

	return nil
}

// clusterHosts returns every ESX host in the cluster (or standalone
// compute resource) that owns the configured resource pool, so a vMotion
// target is guaranteed to be a peer of the VM's current host rather than a
// host in an unrelated cluster the finder happens to also see.
func (r *runner) clusterHosts(ctx context.Context) ([]*object.HostSystem, error) {
	var moPool mo.ResourcePool

	err := r.resourcePool.Properties(ctx, r.resourcePool.Reference(), []string{"owner"}, &moPool)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve resource pool owner: %w", err)
	}

	cr := object.NewComputeResource(r.client.Client, moPool.Owner)

	var moCR mo.ComputeResource

	err = cr.Properties(ctx, cr.Reference(), []string{"host"}, &moCR)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster hosts: %w", err)
	}

	hosts := make([]*object.HostSystem, 0, len(moCR.Host))
	for _, ref := range moCR.Host {
		hosts = append(hosts, object.NewHostSystem(r.client.Client, ref))
	}

	return hosts, nil
}

// hostName returns a host's name for display, falling back to its managed
// object ID when the name can't be read.
func (r *runner) hostName(ctx context.Context, host *object.HostSystem) string {
	var moHost mo.HostSystem

	err := host.Properties(ctx, host.Reference(), []string{"name"}, &moHost)
	if err != nil || moHost.Name == "" {
		return host.Reference().Value
	}

	return moHost.Name
}

// experiment pairs an ID with the function that runs it, so -only and -skip can
// select without the runner knowing what each one does.
type experiment struct {
	id  string
	run func(context.Context) *result
}

// experiments returns every experiment in the order tasks.md T001 lists them.
func (r *runner) experiments() []experiment {
	return []experiment{
		{e01CreateVMExplicit, r.runE01},
		{e02DeployOVFExplicit, r.runE02},
		{e03ReconfigureExplicit, r.runE03},
		{e04ReconfigureAuto, r.runE04},
		{e05AddRemove, r.runE05},
		{e06SameSlotReuse, r.runE06},
		{e07EditUnitNumber, r.runE07},
		{e08Collision, r.runE08},
		{e09ControllerKey, r.runE09},
		{e10NonNICPCIOccupant, r.runE10},
		{e11FreedSlotReuse, r.runE11},
		{e12PowerCycleStability, r.runE12},
		{e13OutOfBandAdd, r.runE13},
		{e14SRIOV, r.runE14},
		{e15HotAddExplicit, r.runE15},
		{e16SuspendResume, r.runE16},
		{e17VMotion, r.runE17},
		{e18HardwareVersionUpgrade, r.runE18},
		{e19OutOfRangeUnitNumber, r.runE19},
	}
}

// selected reports whether an experiment should run under -only and -skip.
func (r *runner) selected(id string) bool {
	if r.cfg.skip != "" && slices.Contains(splitIDs(r.cfg.skip), id) {
		return false
	}

	if r.cfg.only == "" {
		return true
	}

	return slices.Contains(splitIDs(r.cfg.only), id)
}

// splitIDs parses a comma-separated experiment ID list.
func splitIDs(s string) []string {
	parts := strings.Split(s, ",")
	ids := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			ids = append(ids, p)
		}
	}

	return ids
}

// runAll runs every selected experiment. An experiment that fails is recorded
// and the run continues, so one bad environment prerequisite does not cost the
// other fourteen results.
func (r *runner) runAll(ctx context.Context) {
	for _, e := range r.experiments() {
		if !r.selected(e.id) {
			continue
		}

		logf("Running %s ...", e.id)

		res := r.runSafely(ctx, e)
		r.results = append(r.results, res)

		logf("  %s: %s", e.id, res.Status)
	}
}

// runSafely runs one experiment, converting a panic into a recorded error so
// cleanup still happens.
func (r *runner) runSafely(ctx context.Context, e experiment) (res *result) {
	defer func() {
		p := recover()
		if p != nil {
			res = (&result{ID: e.id, Title: "panicked"}).fail(fmt.Errorf("panic: %v", p))
		}
	}()

	return e.run(ctx)
}

// writeReport emits the Markdown report and, when asked, the raw JSON.
func (r *runner) writeReport() error {
	rep := report{Environment: r.env, Results: r.results}
	md := rep.renderMarkdown()

	if r.cfg.outMarkdown == "" {
		fmt.Print(md)
	} else {
		err := os.WriteFile(r.cfg.outMarkdown, []byte(md), 0600)
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", r.cfg.outMarkdown, err)
		}

		logf("Wrote the Markdown report to %s", r.cfg.outMarkdown)
	}

	if r.cfg.outJSON == "" {
		return nil
	}

	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal the results as JSON: %w", err)
	}

	err = os.WriteFile(r.cfg.outJSON, raw, 0600)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", r.cfg.outJSON, err)
	}

	logf("Wrote the raw results to %s", r.cfg.outJSON)

	return nil
}

func main() {
	var cfg config

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	registerFlags(fs, &cfg)

	err := fs.Parse(os.Args[1:])
	if err != nil {
		logf("%v", err)
		os.Exit(1)
	}

	err = run(cfg)
	if err != nil {
		logf("ERROR: %v", err)
		os.Exit(1)
	}
}

// run connects, runs the selected experiments, cleans up, and writes the
// report. Cleanup is deferred so it happens even when an experiment aborts the
// run (R7).
func run(cfg config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := &runner{
		cfg:   cfg,
		runID: strconv.FormatInt(time.Now().Unix(), 36),
	}

	err := r.connect(ctx)
	if err != nil {
		return err
	}

	defer func() {
		logoutErr := r.rest.Logout(ctx)
		if logoutErr != nil {
			logf("WARNING: failed to log out of the REST endpoint: %v", logoutErr)
		}

		logoutErr = r.client.Logout(ctx)
		if logoutErr != nil {
			logf("WARNING: failed to log out of vCenter: %v", logoutErr)
		}
	}()

	r.recordEnvironment(ctx)

	defer r.cleanup(ctx)

	r.runAll(ctx)

	return r.writeReport()
}
