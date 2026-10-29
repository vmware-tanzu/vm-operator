// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Command session-relogin validates the production vCenter client against a
// real testbed. Credentials are read from a private JSON file, never flags.
//
// Inspect: go run ./hack/vcresearch/session-relogin -credentials /private/vc.json
// Exercise: add -exercise -journal /private/new-run-state.json.
// Retry interrupted cleanup: use -cleanup with the same journal.
// Credentials use {"host":"vc.example.test","username":"...","password":"..."}.
// The exercise creates and removes one local SSO user and a ReadOnly permission.
// It does not restart services, modify VMs, or change SSO policies.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/ssoadmin"
	ssomethods "github.com/vmware/govmomi/ssoadmin/methods"
	ssotypes "github.com/vmware/govmomi/ssoadmin/types"
	"github.com/vmware/govmomi/sts"
	"github.com/vmware/govmomi/vim25/soap"
)

type credentials struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type testbed struct {
	credentials credentials
	vc          *govmomi.Client
	sso         *ssoadmin.Client
	datacenter  string
	policy      ssotypes.AdminLockoutPolicy
}

func main() {
	if err := runCommand(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCommand() error {
	credentialsPath := flag.String("credentials", "", "Private JSON file with host, username, and password")
	exercise := flag.Bool("exercise", false, "Create a disposable SSO user and exercise production session recovery")
	journal := flag.String("journal", "", "Private cleanup journal path (required with -exercise)")
	cleanup := flag.Bool("cleanup", false, "Retry cleanup of the test account recorded in -journal")
	flag.Parse()
	interruptCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(interruptCtx, 15*time.Minute)
	defer cancel()
	ctx = logr.NewContext(ctx, logr.Discard())
	switch {
	case *cleanup:
		return cleanupJournal(ctx, *credentialsPath, *journal)
	case *exercise:
		return run(ctx, *credentialsPath, *journal)
	default:
		return inspect(ctx, *credentialsPath)
	}
}

// logout releases admin sessions on a best-effort basis during cleanup.
func (b *testbed) logout() {
	_ = b.sso.Logout(context.Background())
	_ = b.vc.Logout(context.Background())
}

func inspect(ctx context.Context, path string) error {
	b, err := connect(ctx, path)
	if err != nil {
		return err
	}
	defer b.logout()
	passwordPolicy, err := b.sso.GetLocalPasswordPolicy(ctx)
	if err != nil {
		return fmt.Errorf("read password policy: %w", err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"version":         b.vc.ServiceContent.About.Version,
		"build":           b.vc.ServiceContent.About.Build,
		"datacenter":      b.datacenter,
		"sso_domain":      b.sso.Domain,
		"lockout_policy":  b.policy,
		"password_policy": passwordPolicy,
	})
}

func connect(ctx context.Context, path string) (*testbed, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: The CLI operator explicitly chooses the local credentials file.
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	b := &testbed{}
	if err := json.Unmarshal(data, &b.credentials); err != nil {
		return nil, fmt.Errorf("decode credentials: %w", err)
	}
	u := &url.URL{Scheme: "https", Host: b.credentials.Host, Path: "/sdk",
		User: url.UserPassword(b.credentials.Username, b.credentials.Password)}
	b.vc, err = govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, fmt.Errorf("connect admin: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = b.vc.Logout(context.Background())
		}
	}()
	tokens, err := sts.NewClient(ctx, b.vc.Client)
	if err != nil {
		return nil, fmt.Errorf("create STS client: %w", err)
	}
	token, err := tokens.Issue(ctx, sts.TokenRequest{Userinfo: u.User})
	if err != nil {
		return nil, fmt.Errorf("issue SSO token: %w", err)
	}
	b.sso, err = ssoadmin.NewClient(ctx, b.vc.Client)
	if err != nil {
		return nil, fmt.Errorf("create SSO admin client: %w", err)
	}
	if err := b.sso.Login(b.sso.WithHeader(ctx, soap.Header{Security: token})); err != nil {
		return nil, fmt.Errorf("login SSO admin: %w", err)
	}
	defer func() {
		if !ok {
			_ = b.sso.Logout(context.Background())
		}
	}()
	dcs, err := find.NewFinder(b.vc.Client, false).DatacenterList(ctx, "*")
	if err != nil || len(dcs) == 0 {
		return nil, fmt.Errorf("discover datacenter: count=%d, error=%w", len(dcs), err)
	}
	b.datacenter = dcs[0].Reference().Value
	policy, err := ssomethods.GetLockoutPolicy(ctx, b.sso, &ssotypes.GetLockoutPolicy{
		This: b.sso.ServiceContent.LockoutPolicyService,
	})
	if err != nil {
		return nil, fmt.Errorf("read lockout policy: %w", err)
	}
	b.policy = policy.Returnval
	ok = true
	return b, nil
}
