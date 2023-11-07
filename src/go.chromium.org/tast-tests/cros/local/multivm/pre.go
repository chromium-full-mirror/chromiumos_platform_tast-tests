// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package multivm

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/testing"
)

// DefaultChromeOptions defines the default options for creating Chrome.
var DefaultChromeOptions = ChromeOptions{
	Timeout:     chrome.LoginTimeout,
	BrowserType: browser.TypeAsh,
}

// DefaultChromeOptionsVMMMS defines the default options for creating Chrome with
// VMMMS enabled.
var DefaultChromeOptionsVMMMS = ChromeOptions{
	EnableFeatures: []string{"CrOSLateBootVmMemoryManagementService"},
	Timeout:        chrome.LoginTimeout,
	BrowserType:    browser.TypeAsh,
}

// TabManagerDelegateChromeOptionsVMMMS defines the default options for creating
// Chrome with VMMMS enabled and using TabManagerDelegate to discard tabs.
var TabManagerDelegateChromeOptionsVMMMS = ChromeOptions{
	EnableFeatures: []string{"CrOSLateBootVmMemoryManagementService"},
	ExtraArgs:      []string{"--disable-features=AshUrgentDiscardingFromPerformanceManager"},
	Timeout:        chrome.LoginTimeout,
	BrowserType:    browser.TypeAsh,
}

// NoSyncChromeOptions defines special chrome options that prevent background sync.
var NoSyncChromeOptions = ChromeOptions{
	Timeout:     chrome.LoginTimeout,
	BrowserType: browser.TypeAsh,
	ExtraArgs:   arc.DisableSyncFlags(),
}

// LacrosChromeOptions creates Lacros Chrome.
var LacrosChromeOptions = ChromeOptions{
	Timeout:     chrome.LoginTimeout,
	BrowserType: browser.TypeLacros,
}

// LacrosChromeOptionsVmms creates Lacros Chrome with VMMMS enabled.
var LacrosChromeOptionsVmms = ChromeOptions{
	EnableFeatures: []string{"CrOSLateBootVmMemoryManagementService"},
	Timeout:        chrome.LoginTimeout,
	BrowserType:    browser.TypeLacros,
}

// DefaultARCOptions defines the default options for starting ARC VM.
var DefaultARCOptions = ARCOptions{}

// DefaultCrostiniOptions defines the default options for starting Crostini.
var DefaultCrostiniOptions = CrostiniOptions{
	LargeContainer: false,
	DebianVersion:  vm.DebianBullseye,
}

var noVMStartedPre = NewMultiVMPrecondition(
	"multivm_no_vm",
	NewStateManager(
		DefaultChromeOptions,
	).SetForceActivate(true))

// NoVMStarted returns a Precondition that logs into Chrome without starting any
// VMs.
func NoVMStarted() testing.Precondition {
	return noVMStartedPre
}

var noVMLacrosStartedPre = NewMultiVMPrecondition(
	"multivm_no_vm_lacros",
	NewStateManager(
		LacrosChromeOptions,
	).SetForceActivate(true))

// NoVMLacrosStarted returns a Precondition that logs into Lacros Chrome without
// starting any VMs.
func NoVMLacrosStarted() testing.Precondition {
	return noVMLacrosStartedPre
}

var arcCrostiniStartedPre = NewMultiVMPrecondition(
	"multivm_arc_crostini",
	NewStateManager(
		DefaultChromeOptions,
		DefaultARCOptions,
		DefaultCrostiniOptions,
	))

// ArcCrostiniStarted returns a Precondition that logs into Chrome and starts
// ARCVM an Crostini.
func ArcCrostiniStarted() testing.Precondition {
	return arcCrostiniStartedPre
}

var arcCrostiniLacrosStartedPre = NewMultiVMPrecondition(
	"multivm_arc_crostini_lacros",
	NewStateManager(
		LacrosChromeOptions,
		DefaultARCOptions,
		DefaultCrostiniOptions,
	))

// ArcCrostiniLacrosStarted returns a Precondition that logs into Lacros Chrome
// and starts ARCVM an Crostini.
func ArcCrostiniLacrosStarted() testing.Precondition {
	return arcCrostiniLacrosStartedPre
}

var arcStartedPre = NewMultiVMPrecondition(
	"multivm_arc",
	NewStateManager(
		DefaultChromeOptions,
		DefaultARCOptions,
	).SetForceActivate(true))

// ArcStarted returns a Precondition that logs into Chrome and starts ARCVM.
func ArcStarted() testing.Precondition {
	return arcStartedPre
}

var arcStartedVMMMSPre = NewMultiVMPrecondition(
	"multivm_arc_vmmms",
	NewStateManager(
		DefaultChromeOptionsVMMMS,
		DefaultARCOptions,
	).SetForceActivate(true))

// ArcStartedVMMMS returns a Precondition that logs into Chrome with VMMMS
// enabled and starts ARCVM.
func ArcStartedVMMMS() testing.Precondition {
	return arcStartedVMMMSPre
}

var arcStartedVMMMSTabManagerDelegatePre = NewMultiVMPrecondition(
	"multivm_arc_vmmms_tmd",
	NewStateManager(
		TabManagerDelegateChromeOptionsVMMMS,
		DefaultARCOptions,
	).SetForceActivate(true))

// ArcStartedVMMMSTabManagerDelegate returns a Precondition that logs into
// Chrome with VMMMS enabled and using TabManagerDelegate to discard tabs and
// starts ARCVM.
func ArcStartedVMMMSTabManagerDelegate() testing.Precondition {
	return arcStartedVMMMSPre
}

var arcStartedNoSyncPre = NewMultiVMPrecondition(
	"multivm_arc_nosync",
	NewStateManager(
		NoSyncChromeOptions,
		DefaultARCOptions,
	).SetForceActivate(true))

// ArcStartedNoSync returns a Precondition that logs into Chrome and starts ARCVM.
func ArcStartedNoSync() testing.Precondition {
	return arcStartedNoSyncPre
}

var arcLacrosStartedPre = NewMultiVMPrecondition(
	"multivm_arc_lacros",
	NewStateManager(
		LacrosChromeOptions,
		DefaultARCOptions,
	).SetForceActivate(true))

// ArcLacrosStarted returns a Precondition that logs into Lacros Chrome and
// starts ARCVM.
func ArcLacrosStarted() testing.Precondition {
	return arcLacrosStartedPre
}

var arcLacrosStartedVMMMSPre = NewMultiVMPrecondition(
	"multivm_arc_lacros_vmmms",
	NewStateManager(
		LacrosChromeOptionsVmms,
		DefaultARCOptions,
	).SetForceActivate(true))

// ArcLacrosStartedVMMMS returns a Precondition that logs into Lacros Chrome
// with VMMMS enabled and starts ARCVM.
func ArcLacrosStartedVMMMS() testing.Precondition {
	return arcLacrosStartedVMMMSPre
}

var crostiniStartedPre = NewMultiVMPrecondition(
	"multivm_crostini",
	NewStateManager(
		DefaultChromeOptions,
		DefaultCrostiniOptions,
	))

// CrostiniStarted returns a Precondition that logs into Chrome and starts
// Crostini.
func CrostiniStarted() testing.Precondition {
	return crostiniStartedPre
}

var crostiniLacrosStartedPre = NewMultiVMPrecondition(
	"multivm_crostini_lacros",
	NewStateManager(
		LacrosChromeOptions,
		DefaultCrostiniOptions,
	))

// CrostiniLacrosStarted returns a Precondition that logs into Lacros Chrome and
// starts Crostini.
func CrostiniLacrosStarted() testing.Precondition {
	return crostiniLacrosStartedPre
}

type preImpl struct {
	// Configuration.
	name    string // testing.Precondition.String
	vmState StateManager
}

// PreData holds data allowing tests to interact with the VMs requested by their
// precondition.
type PreData struct {
	// Always available.
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	Keyboard    *input.KeyboardEventWriter
	// The VMs set up by the precondition. It is recommended to access via the
	// VM-defined helper methods, such as multivm.ARCFromPre and
	// multivm.CrostiniFromPre.
	VMs map[string]interface{}
}

// NewMultiVMPrecondition returns a new precondition that can be used be
// used by tests that expect multiple VMs to be started at the start of the
// test.
func NewMultiVMPrecondition(name string, vmState *StateManager) testing.Precondition {
	return &preImpl{
		name:    name,
		vmState: *vmState,
	}
}

func (p *preImpl) String() string { return p.name }
func (p *preImpl) Timeout() time.Duration {
	return p.vmState.Timeout()
}

func (p *preImpl) preData() *PreData {
	return &PreData{
		Chrome:      p.vmState.Chrome(),
		TestAPIConn: p.vmState.TestAPIConn(),
		Keyboard:    p.vmState.Keyboard(),
		VMs:         p.vmState.VMs(),
	}
}

// Called by tast before each test is run. We use this method to initialize
// the precondition data, or return early if the precondition is already
// active.
func (p *preImpl) Prepare(ctx context.Context, s *testing.PreState) interface{} {
	if p.vmState.Active() {
		if !p.vmState.IsForceActivateEnabled() {
			err := p.vmState.CheckAndReset(ctx, s)
			if err == nil {
				return p.preData()
			}
			s.Log("Failed checking or resetting Chrome+VMs: ", err)
		} else {
			s.Log("Already-active precondition forced to re-activate")
		}
		if err := p.vmState.Deactivate(ctx); err != nil {
			s.Fatal("Failed to deactivate Chrome+VMs after check failed: ", err)
		}
	}
	if err := p.vmState.Activate(ctx, s); err != nil {
		s.Fatal("Failed to activate Chrome+VMs: ", err)
	}
	return p.preData()
}

// Close is called after all tests involving this precondition have been run.
// Stops all requested VMs and Chrome.
func (p *preImpl) Close(ctx context.Context, s *testing.PreState) {
	if err := p.vmState.Deactivate(ctx); err != nil {
		s.Fatal("Failed to deacivate Chrome+VMs: ", err)
	}
}
