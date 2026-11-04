// Copyright (c) 2026 Broadcom. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRunner is a dcli.DCLICommandRunner that records commands and replays
// canned results.
type fakeRunner struct {
	cmds    []string
	results []fakeResult
}

type fakeResult struct {
	out []byte
	err error
}

func (f *fakeRunner) RunDCLICommand(cmd string) ([]byte, error) {
	f.cmds = append(f.cmds, cmd)

	if len(f.results) == 0 {
		return nil, nil
	}

	r := f.results[0]
	if len(f.results) > 1 {
		f.results = f.results[1:]
	}

	return r.out, r.err
}

func (f *fakeRunner) RunCommandAndUnmarshalJSONResult(string, any) error {
	return errors.New("unused")
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"":        `''`,
		"abc":     `'abc'`,
		"a b":     `'a b'`,
		"it's":    `'it'\''s'`,
		"$(rm x)": `'$(rm x)'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestVMReservationsMarshalsEmptyAsArray(t *testing.T) {
	got, err := jsonArg(vmReservations(nil))
	if err != nil {
		t.Fatal(err)
	}

	if want := `'[]'`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestRunWrapsErrorWithResponse(t *testing.T) {
	d := &wcpDcliClient{dcliClient: &fakeRunner{results: []fakeResult{{out: []byte("boom"), err: errors.New("failed")}}}}

	var dcliErr DcliError
	if err := d.runVoid("x"); !errors.As(err, &dcliErr) || dcliErr.Response() != "boom" {
		t.Errorf("expected DcliError with response, got %v", err)
	}
}

func TestRunIgnoringAlreadyExists(t *testing.T) {
	d := &wcpDcliClient{dcliClient: &fakeRunner{results: []fakeResult{{out: []byte("errors.AlreadyExists"), err: errors.New("failed")}}}}
	if err := d.runIgnoringAlreadyExists("x"); err != nil {
		t.Errorf("expected AlreadyExists to be ignored, got %v", err)
	}

	d = &wcpDcliClient{dcliClient: &fakeRunner{results: []fakeResult{{out: []byte("other"), err: errors.New("failed")}}}}
	if err := d.runIgnoringAlreadyExists("x"); err == nil {
		t.Error("expected other errors to be returned")
	}
}

func TestRetry(t *testing.T) {
	f := &fakeRunner{results: []fakeResult{
		{err: errors.New("not yet")},
		{err: errors.New("not yet")},
		{},
	}}
	d := &wcpDcliClient{dcliClient: f}

	if err := d.retry("x", time.Millisecond, time.Second); err != nil {
		t.Fatal(err)
	}

	if len(f.cmds) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(f.cmds))
	}

	f = &fakeRunner{results: []fakeResult{{err: errors.New("always")}}}
	d = &wcpDcliClient{dcliClient: f}

	if err := d.retry("x", time.Millisecond, 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "always") {
		t.Errorf("expected last error, got %v", err)
	}
}

func TestClusterContentLibrariesUseEmptySupervisorServices(t *testing.T) {
	f := &fakeRunner{}
	d := &wcpDcliClient{dcliClient: f}

	err := d.updateClusterContentLibraries("domain-c1", []ClusterContentLibrarySpec{{ContentLibrary: "cl-1", ResourceNamingStrategy: "FROM_ITEM_ID"}})
	if err != nil {
		t.Fatal(err)
	}

	want := `--content-libraries '[{"content_library":"cl-1","supervisor_services":[],"resource_naming_strategy":"FROM_ITEM_ID"}]'`
	if !strings.Contains(f.cmds[0], want) {
		t.Errorf("command %q does not contain %q", f.cmds[0], want)
	}
}

func TestVMClassSpecCmdQuotesDescription(t *testing.T) {
	cpu, mem, desc := 2, 1024, `it's "fun"`

	cmd, err := GenerateVMClassSpecCmd(VMClassSpec{CPUCount: &cpu, MemoryMB: &mem, Description: &desc})
	if err != nil {
		t.Fatal(err)
	}

	if want := `--description 'it'\''s "fun"'`; !strings.Contains(cmd, want) {
		t.Errorf("command %q does not contain %q", cmd, want)
	}
}
