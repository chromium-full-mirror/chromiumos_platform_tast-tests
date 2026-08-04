// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/pvsched"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/timing"
)

// ResetTimeout is the timeout duration to trying reset of the current precondition.
const ResetTimeout = 30 * time.Second

// ArcBootHwDepConds is a list of the minimum hardware conditions to run ARC tests without additional harnesses (eg. UIAutomator, CTS, GTS, ...).
var ArcBootHwDepConds = []hwdep.Condition{hwdep.MinStorage(16)}

// ArcBootHwDeps is a minimum hardware dependency for ARC tests.
var ArcBootHwDeps = hwdep.D(ArcBootHwDepConds...)

// ArcAppHwDepConds is a list of the hardware conditions of the boards on which the ARC Apps tests run.
// ARC testing uses a large storage for apps and harnesses (eg. UIAutomator, CTS, GTS, ...). Not all DUTs in the lab do not have enough space.
var ArcAppHwDepConds = []hwdep.Condition{hwdep.MinStorage(24)}

// ArcAppHwDeps is a hardware dependency limiting the boards on which the ARC Apps tests run.
var ArcAppHwDeps = hwdep.D(ArcAppHwDepConds...)

// PreData holds information made available to tests that specify preconditions.
type PreData struct {
	// parentState is the parent state that provides extra functionality, such as multi display toggling.
	parentState interface{}
	// Chrome is a connection to an already-started Chrome instance.
	// It cannot be closed by tests.
	Chrome *chrome.Chrome
	// ARC enables interaction with an already-started ARC environment.
	// It cannot be closed by tests.
	ARC *ARC
	// UIDevice is a UI Automator device object.
	// It cannot be closed by tests.
	UIDevice *ui.Device
}

// Booted returns a precondition that ARC Container has already booted when a test is run.
//
// When adding a test, the testing.Test.Pre field may be set to the value returned by this function.
// Later, in the main test function, the value returned by testing.State.PreValue may be converted
// to a PreData containing already-initialized chrome.Chrome and ARC objects:
//
//	func DoSomething(ctx context.Context, s *testing.State) {
//		d := s.PreValue().(arc.PreData)
//		conn, err := d.Chrome.NewConn(ctx, "http://www.example.org/")
//		...
//		cmd := d.ARC.Command(ctx, "dumpsys", "window", "displays")
//		...
//	}
//
// When using this precondition, tests cannot call New or chrome.New.
// The Chrome and ARC instances are also shared and cannot be closed by tests.
func Booted() testing.Precondition { return bootedPre }

// bootedPre is returned by Booted.
var bootedPre = &preImpl{
	name:    "arc_booted",
	timeout: ResetTimeout + chrome.LoginTimeout + BootTimeout,
}

// NewPrecondition creates a new arc precondition for tests that need different args.
func NewPrecondition(name string, gaia *GaiaVars, gaiaPool *GaiaLoginPoolVars, rootfsODirect bool, extraArgs ...string) testing.Precondition {
	timeout := ResetTimeout + chrome.LoginTimeout + BootTimeout
	if gaia != nil || gaiaPool != nil {
		timeout = ResetTimeout + chrome.GAIALoginTimeout + BootTimeout + optin.OptinTimeout
	}
	pre := &preImpl{
		name:          name,
		timeout:       timeout,
		gaia:          gaia,
		gaiaPool:      gaiaPool,
		extraArgs:     extraArgs,
		rootfsODirect: rootfsODirect,
	}
	return pre
}

// NewPreconditionWithPvSchedEnabled creates a new arc precondition for tests that need different args.
// Similar to NewPrecondition, but enabled paravirt sched feature.
// TODO(b/325918094): Remove when paravirt sched is fully enabled on chromeos
func NewPreconditionWithPvSchedEnabled(name string, gaia *GaiaVars, gaiaPool *GaiaLoginPoolVars, rootfsODirect bool, extraArgs ...string) testing.Precondition {
	timeout := ResetTimeout + chrome.LoginTimeout + BootTimeout
	if gaia != nil || gaiaPool != nil {
		timeout = ResetTimeout + chrome.GAIALoginTimeout + BootTimeout + optin.OptinTimeout
	}
	pre := preImpl{
		name:          name,
		timeout:       timeout,
		gaia:          gaia,
		gaiaPool:      gaiaPool,
		extraArgs:     extraArgs,
		rootfsODirect: rootfsODirect,
	}
	return &preImplWithPvSchedEnabled{
		preImpl: pre,
	}
}

// GaiaVars holds the secret variables for username and password for a GAIA login.
type GaiaVars struct {
	UserVar string // the secret variable for the GAIA username
	PassVar string // the secret variable for the GAIA password
}

// GaiaLoginPoolVars holds the secret variable for a GAIA POOL.
type GaiaLoginPoolVars struct {
	PoolVar string // the secret variable for the GAIA pool
}

// preImpl implements testing.Precondition.
type preImpl struct {
	name string // testing.Precondition.String

	timeout time.Duration // testing.Precondition.Timeout

	extraArgs     []string           // passed to Chrome on initialization
	gaia          *GaiaVars          // a struct containing GAIA secret variables
	gaiaPool      *GaiaLoginPoolVars // a struct containing GAIA login pool variable
	rootfsODirect bool               // whether crosvm should use O_DIRECT for read-only system/vendor file systems.
	cr            *chrome.Chrome
	arc           *ARC

	init *Snapshot
}

func (p *preImpl) String() string         { return p.name }
func (p *preImpl) Timeout() time.Duration { return p.timeout }

// Prepare is called by the test framework at the beginning of every test using this precondition.
// It returns a PreData containing objects that can be used by the test.
func (p *preImpl) Prepare(ctx context.Context, s *testing.PreState) interface{} {
	ctx, st := timing.Start(ctx, "prepare_"+p.name)
	defer st.End()

	if p.arc != nil {
		pre, err := func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(ctx, ResetTimeout)
			defer cancel()
			ctx, st := timing.Start(ctx, "reset_"+p.name)
			defer st.End()
			if err := p.init.Restore(ctx, p.arc); err != nil {
				return nil, errors.Wrap(err, "failed to reset ARC")
			}
			if err := p.cr.ResetState(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to reset Chrome")
			}
			if err := p.arc.SaveLogFiles(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to save ARC-related log files")
			}
			if err := p.arc.ResetOutDir(ctx, s.OutDir()); err != nil {
				return nil, errors.Wrap(err, "failed to reset outDir field of ARC object")
			}
			return PreData{Chrome: p.cr, ARC: p.arc}, nil
		}()
		if err == nil {
			s.Log("Reusing existing ARC session")
			return pre
		}
		s.Log("Failed to reuse existing ARC session: ", err)
		Unlock()
		chrome.Unlock()
		p.closeInternal(ctx, s)
	}

	// Revert partial initialization.
	shouldClose := true
	defer func() {
		if shouldClose {
			p.closeInternal(ctx, s)
		}
	}()

	func() {
		ctx, cancel := context.WithTimeout(ctx, chrome.LoginTimeout)
		defer cancel()
		extraArgs := p.extraArgs
		var err error

		if p.rootfsODirect {
			// Set up O_DIRECT for /dev/vda (system.img) and /dev/vdb (vendor.img).
			if err := WriteArcvmDevConf(ctx, "O_DIRECT_N=0\nO_DIRECT_N=1\n"); err != nil {
				s.Fatal("Failed to set arcvm_dev.conf: ", err)
			}
		} else {
			if err := WriteArcvmDevConf(ctx, ""); err != nil {
				s.Fatal("Failed to set arcvm_dev.conf: ", err)
			}
		}

		if p.gaia != nil {
			// Login into the device, using GAIA login.
			username := s.RequiredVar(p.gaia.UserVar)
			password := s.RequiredVar(p.gaia.PassVar)

			p.cr, err = chrome.New(
				ctx,
				chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
				chrome.ARCSupported(),
				chrome.ExtraArgs(extraArgs...))
		} else if p.gaiaPool != nil {
			// Login into the device, using GAIA login pool.
			p.cr, err = chrome.New(
				ctx,
				chrome.GAIALoginPool(s.RequiredVar(p.gaiaPool.PoolVar)),
				chrome.ARCSupported(),
				chrome.ExtraArgs(extraArgs...))
		} else {
			p.cr, err = chrome.New(
				ctx,
				chrome.ARCEnabled(),
				chrome.ExtraArgs(extraArgs...))
		}
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
	}()

	// Opt-in to playstore using GAIA login or GAIA login pool.
	if p.gaia != nil || p.gaiaPool != nil {
		func() {
			ctx, cancel := context.WithTimeout(ctx, optin.OptinTimeout)
			defer cancel()
			tconn, err := p.cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to create test API connection: ", err)
			}
			if err := optin.PerformAndClose(ctx, p.cr, tconn); err != nil {
				s.Fatal("Failed to optin to Play Store and Close: ", err)
			}
		}()
	}

	func() {
		ctx, cancel := context.WithTimeout(ctx, BootTimeout)
		defer cancel()
		var err error
		if p.arc, err = New(ctx, s.OutDir(), p.cr.NormalizedUser()); err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		if p.init, err = NewSnapshot(ctx, p.arc); err != nil {
			s.Fatal("Failed to get initial ARC state snapshot: ", err)
		}
	}()

	// Prevent the arc and chrome package's New and Close functions from
	// being called while this precondition is active.
	Lock()
	chrome.Lock()

	shouldClose = false
	return PreData{Chrome: p.cr, ARC: p.arc}
}

// Close is called by the test framework after the last test that uses this precondition.
func (p *preImpl) Close(ctx context.Context, s *testing.PreState) {
	ctx, st := timing.Start(ctx, "close_"+p.name)
	defer st.End()

	Unlock()
	chrome.Unlock()
	p.closeInternal(ctx, s)
}

// closeInternal closes and resets p.arc and p.cr if non-nil.
func (p *preImpl) closeInternal(ctx context.Context, s *testing.PreState) {
	if p.arc != nil {
		if err := p.arc.Close(ctx); err != nil {
			s.Log("Failed to close ARC connection: ", err)
		}
		p.arc = nil
	}
	if p.cr != nil {
		if err := p.cr.Close(ctx); err != nil {
			s.Log("Failed to close Chrome connection: ", err)
		}
		p.cr = nil
	}
	p.init = nil
	if err := RestoreArcvmDevConf(ctx); err != nil {
		s.Log("Failed to write arcvm_dev.conf: ", err)
	}
}

// Similar to preImpl, but with paravirt sched feature enabled.
// TODO(b/325918094): Remove when paravirt sched is fully enabled on chromeos
type preImplWithPvSchedEnabled struct {
	pvSchedEnabled bool // Current state of PvSched feature
	preImpl
}

func (p *preImplWithPvSchedEnabled) String() string         { return p.preImpl.name }
func (p *preImplWithPvSchedEnabled) Timeout() time.Duration { return p.preImpl.timeout }

func (p *preImplWithPvSchedEnabled) Prepare(ctx context.Context, s *testing.PreState) interface{} {
	var err error

	p.pvSchedEnabled, err = pvsched.Enabled()
	if err != nil {
		s.Fatal("Failed to parse Paravirt Sched feature state: ", err)
	}

	if !p.pvSchedEnabled {
		pvsched.Enable()
	}

	return p.preImpl.Prepare(ctx, s)
}

func (p *preImplWithPvSchedEnabled) Close(ctx context.Context, s *testing.PreState) {
	if !p.pvSchedEnabled {
		pvsched.Disable()
	}

	p.preImpl.Close(ctx, s)
}
