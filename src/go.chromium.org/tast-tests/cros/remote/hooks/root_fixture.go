// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            testing.TastRootRemoteFixtureName,
		Desc:            "Root fixture that will be run before all tests and fixtures",
		Contacts:        []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent:    "b:1034754",
		Impl:            &rootFixture{},
		SetUpTimeout:    5 * time.Minute,
		ResetTimeout:    2 * time.Minute,
		PreTestTimeout:  2 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		TearDownTimeout: 5 * time.Minute,
		ServiceDeps: []string{
			"tast.cros.cleanupchecks.CleanUpChecksService",
			"tast.cros.actionlogger.ActionLoggerService",
		},
		Vars: []string{
			"servo", // Need by servo hook. TODO: remove after replacing it with servers.servo.
		},
	})
}

// HookImpl is an interface that will be used by rootFixture in each step of
// fixture operation.
type HookImpl interface {
	// SetUp will be called during root fixture Setup.
	SetUp(ctx context.Context, s *HookState) error

	// Reset will be called during root fixture Reset.
	Reset(ctx context.Context) error

	// PreTest will be called during root fixture PreTest.
	PreTest(ctx context.Context, s *HookTestState) error

	// PostTest will be called during root fixture PostTest.
	PostTest(ctx context.Context, s *HookTestState) error

	// TearDown will be called during root fixture TearDown.
	TearDown(ctx context.Context, s *HookState) error
}

// Hook is the structure for a hook.
type Hook struct {
	Name         string   // The name of the hook.
	Desc         string   // The description of the hook.
	Contacts     []string // The aliases and emails of the owners of the hook.
	BugComponent string   // The bug component of the hook.
	Impl         HookImpl // The implementation of the hook.
}

// hooks maintains a mapping between the name of a hook
// and a hook.
var hooks map[string]*Hook

// orderHooks maintains a list of names of hooks.
// The root remote fixture will execute hooks by the
// order of this list during Setup, PreTest and Reset.
// On the other hand, PostTest and TearDown will be
// executed in the reverse order of this list.
// IMPORTANT: a hook will not be executed until the name
// is added to this list.
var orderedHooks []string = []string{
	"servoHook",
	"kdumpHook",
	"diskThrottler",
	"cleanupChecksHook",
	"actionLoggerHook",
	"cujCleanup",
	"debugInfoHook", // Prefer to run this hook at the end if possible.
}

// HookState includes certain fixture state information that is
// available for each hook to be used during Setup and Teardown.
type HookState struct {
	fixtState *testing.FixtState
	hookName  string
}

// FixtContext returns fixture-scoped context. i.e. the context is alive until TearDown returns.
// The context is also associated with the fixture metadata. For example,
// testing.ContextOutDir(ctx) returns the output directory allocated to the fixture.
func (hs *HookState) FixtContext() context.Context { return hs.fixtState.FixtContext() }

// OutDir returns a directory into which the entity may place arbitrary files
// that should be included with the entity results.
func (hs *HookState) OutDir() string { return filepath.Join(hs.fixtState.OutDir(), hs.hookName) }

// CloudStorage returns a client for Google Cloud Storage.
func (hs *HookState) CloudStorage() *testing.CloudStorage { return hs.fixtState.CloudStorage() }

// RPCHint returns information needed to establish gRPC connections.
func (hs *HookState) RPCHint() *testing.RPCHint { return hs.fixtState.RPCHint() }

// DUT returns a shared SSH connection.
func (hs *HookState) DUT() *dut.DUT { return hs.fixtState.DUT() }

// KeyFile returns an optional path to an unencrypted SSH private key.
func (hs *HookState) KeyFile() string { return hs.fixtState.KeyFile() }

// KeyDir returns an optional path to a directory containing standard
// SSH keys to use if authentication via KeyFile is not accepted.
func (hs *HookState) KeyDir() string { return hs.fixtState.KeyDir() }

// CompanionDUT returns a shared SSH connection for a companion DUT.
func (hs *HookState) CompanionDUT(role string) *dut.DUT { return hs.fixtState.CompanionDUT(role) }

// CompanionDUTs returns Companion DUTs added through the command line.
// It can only be called by remote entities.
func (hs *HookState) CompanionDUTs() map[string]*dut.DUT { return hs.fixtState.CompanionDUTs() }

// ChromeOSDUTLabConfig returns the lab configuration of a ChromeOS DUT.
// The role for primary DUT is "".
func (hs *HookState) ChromeOSDUTLabConfig(role string) (*api.Dut, error) {
	return hs.fixtState.ChromeOSDUTLabConfig(role)
}

// AndroidDUTLabConfig returns the lab configuration of an Android DUT.
func (hs *HookState) AndroidDUTLabConfig(associateHostname string) (*api.Dut, error) {
	return hs.fixtState.AndroidDUTLabConfig(associateHostname)
}

// DevboardDUTLabConfig returns the lab configuration of a Devboard DUT.
func (hs *HookState) DevboardDUTLabConfig(role string) (*api.Dut, error) {
	return hs.fixtState.DevboardDUTLabConfig(role)
}

// Var returns the value for the named variable, which must have been registered via Vars.
// If a value was not supplied at runtime via the -var flag to "tast run", ok will be false.
// TODO: remove after all usage of variables are replace by global run time variables.
func (hs *HookState) Var(name string) (val string, ok bool) {
	return hs.fixtState.Var(name)
}

// DataFileSystem returns an http.FileSystem implementation that serves an entity's data files.
//
//	srv := httptest.NewServer(http.FileServer(s.DataFileSystem()))
//	defer srv.Close()
//	resp, err := http.Get(srv.URL+"/data_file.html")
func (hs *HookState) DataFileSystem() http.FileSystem { return hs.fixtState.DataFileSystem() }

// HookTestState includes fixture state information that is available for each hook
// to be used during PreTest and PostTest.
type HookTestState struct {
	fixtTestState *testing.FixtTestState
	hookName      string
}

// OutDir returns a directory into which the entity may place arbitrary files
// that should be included with the entity results.
func (hs *HookTestState) OutDir() string {
	return filepath.Join(hs.fixtTestState.OutDir(), hs.hookName)
}

// TestContext returns context associated with the test.
// It has the same lifetime as the test (including PreTest and PostTest), and
// the same metadata as the ctx passed to PreTest and PostTest.
func (hs *HookTestState) TestContext() context.Context {
	return hs.fixtTestState.TestContext()
}

// TestName returns the test name of the test.
func (hs *HookTestState) TestName() string {
	return hs.fixtTestState.TestName()
}

// CloudStorage returns a client for Google Cloud Storage.
func (hs *HookTestState) CloudStorage() *testing.CloudStorage { return hs.fixtTestState.CloudStorage() }

// RPCHint returns information needed to establish gRPC connections.
func (hs *HookTestState) RPCHint() *testing.RPCHint { return hs.fixtTestState.RPCHint() }

// DUT returns a shared SSH connection.
func (hs *HookTestState) DUT() *dut.DUT { return hs.fixtTestState.DUT() }

// CompanionDUT returns a shared SSH connection for a companion DUT.
func (hs *HookTestState) CompanionDUT(role string) *dut.DUT {
	return hs.fixtTestState.CompanionDUT(role)
}

// CompanionDUTs returns Companion DUTs added through the command line.
// It can only be called by remote entities.
func (hs *HookTestState) CompanionDUTs() map[string]*dut.DUT { return hs.fixtTestState.CompanionDUTs() }

// ChromeOSDUTLabConfig returns the lab configuration of a ChromeOS DUT.
// The role for primary DUT is "".
func (hs *HookTestState) ChromeOSDUTLabConfig(role string) (*api.Dut, error) {
	return hs.fixtTestState.ChromeOSDUTLabConfig(role)
}

// AndroidDUTLabConfig returns the lab configuration of an Android DUT.
func (hs *HookTestState) AndroidDUTLabConfig(associateHostname string) (*api.Dut, error) {
	return hs.fixtTestState.AndroidDUTLabConfig(associateHostname)
}

// DevboardDUTLabConfig returns the lab configuration of a Devboard DUT.
func (hs *HookTestState) DevboardDUTLabConfig(role string) (*api.Dut, error) {
	return hs.fixtTestState.DevboardDUTLabConfig(role)
}

// addHook adds a new hook to the root fixture.
// We intentionally do not expose the symbol of this function
// so that all hooks will be written in this package.
func addHook(h *Hook) {
	if h == nil {
		panic("Cannot add nil as hook")
	}
	if h.Name == "" {
		panic("A hook must have an non-empty name")
	}
	if h.Desc == "" {
		panic("A hook must have a description")
	}
	if len(h.Contacts) == 0 {
		panic("A hook much have a list of valid contacts")
	}
	if h.BugComponent == "" {
		panic("A hook must have a bug component")
	}
	if hooks == nil {
		hooks = make(map[string]*Hook)
	}
	if _, ok := hooks[h.Name]; ok {
		panic(fmt.Sprintf("Duplicate hook %s", h.Name))
	}
	hooks[h.Name] = h
}

type rootFixture struct {
	hooks []*Hook
}

func (rf *rootFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Order all running hook implements.
	if len(rf.hooks) == 0 {
		for _, name := range orderedHooks {
			h, ok := hooks[name]
			if !ok {
				s.Fatal("Undefined hook ", name)
			}
			rf.hooks = append(rf.hooks, h)
		}
	}
	for _, h := range rf.hooks {
		hs := &HookState{
			fixtState: s,
			hookName:  h.Name,
		}
		startTime := time.Now()
		if err := h.Impl.SetUp(ctx, hs); err != nil {
			s.Fatalf("Failed to run Setup for hook %s: %v ms", h.Name, err)
		}
		s.Logf("Remote root fixture hook %s completed Setup in %dms",
			h.Name, time.Since(startTime).Milliseconds())
	}
	return nil
}
func (rf *rootFixture) Reset(ctx context.Context) error {
	for _, h := range rf.hooks {
		startTime := time.Now()
		if err := h.Impl.Reset(ctx); err != nil {
			errors.Wrapf(err, "failed to run Reset for hook %s", h.Name)
		}
		testing.ContextLogf(ctx, "Remote root fixture hook %s completed Reset in %dms",
			h.Name, time.Since(startTime).Milliseconds())
	}
	return nil
}

func (rf *rootFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	for _, h := range rf.hooks {
		hs := &HookTestState{
			fixtTestState: s,
			hookName:      h.Name,
		}
		startTime := time.Now()
		if err := h.Impl.PreTest(ctx, hs); err != nil {
			s.Errorf("Failed to run PreTest for hook %s before running test %s: %v",
				h.Name, s.TestName(), err)
		}
		s.Logf("Remote root fixture hook %s completed PreTest for %s in %dms",
			h.Name, s.TestName(), time.Since(startTime).Milliseconds())
	}
}

func (rf *rootFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Hooks in PostTest will be executed in reverse order.
	for i := len(rf.hooks) - 1; i >= 0; i-- {
		h := rf.hooks[i]
		hs := &HookTestState{
			fixtTestState: s,
			hookName:      h.Name,
		}
		startTime := time.Now()
		if err := h.Impl.PostTest(ctx, hs); err != nil {
			s.Errorf("Failed to run PostTest for hook %s before running test %s: %v",
				s.TestName(), h.Name, err)
		}
		s.Logf("Remote root fixture hook %s completed PostTest for %s in %dms",
			h.Name, s.TestName(), time.Since(startTime).Milliseconds())
	}
}

func (rf *rootFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// Hooks in TearDown will be executed in reverse order.
	for i := len(rf.hooks) - 1; i >= 0; i-- {
		h := rf.hooks[i]
		hs := &HookState{
			fixtState: s,
			hookName:  h.Name,
		}
		startTime := time.Now()
		if err := h.Impl.TearDown(ctx, hs); err != nil {
			s.Logf("Failed to run TearDown for hook %s: %v", h.Name, err)
		}
		s.Logf("Remote root fixture hook %s completed TearDown in %dms",
			h.Name, time.Since(startTime).Milliseconds())
	}
}
