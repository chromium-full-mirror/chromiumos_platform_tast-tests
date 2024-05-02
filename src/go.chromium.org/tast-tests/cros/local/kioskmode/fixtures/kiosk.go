// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixtures contains fixtures useful for Kiosk mode tests.
package fixtures

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	const setupTimeout = kioskmode.SetupDuration + kioskmode.LaunchDuration
	const resetTimeout = chrome.ResetTimeout
	const tearDownTimeout = 5*time.Second + kioskmode.CleanupDuration // Chrome unlock time + kiosk clean up time.
	const postTestTimeout = 15 * time.Second

	testing.AddFixture(&testing.Fixture{
		Name:            fixture.KioskLoggedInAsh,
		Desc:            "Kiosk mode started with default app setup, DUT is enrolled",
		Contacts:        []string{"kamilszarek@google.com", "alt-modalities-stability@google.com"},
		BugComponent:    "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Impl:            &kioskFixture{autoLaunchKioskAppID: kioskmode.WebKioskAccountID},
		SetUpTimeout:    setupTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:         fixture.KioskLoggedInLacros,
		Desc:         "Kiosk mode started with default app setup, DUT is enrolled and Lacros enabled",
		Contacts:     []string{"irfedorova@google.com", "chromeos-kiosk-eng@google.com"},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Impl: &kioskFixture{
			autoLaunchKioskAppID: kioskmode.WebKioskAccountID,
			extraPublicAccountPolicies: []policy.Policy{
				&policy.LacrosAvailability{Val: "lacros_only"},
			},
			lacros: true,
		},
		SetUpTimeout:    setupTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

type kioskFixture struct {
	// cr is a connection to an already-started Chrome instance that loads policies from FakeDMS.
	cr *chrome.Chrome
	// fdms is the already running DMS server from the parent fixture.
	fdms *fakedms.FakeDMS
	// autoLaunchKioskAppID is a preselected Kiosk app ID used for autolaunch.
	autoLaunchKioskAppID string
	// extraPublicAccountPolicies holds a policies that will be applied.
	extraPublicAccountPolicies []policy.Policy
	// proc is the root Chrome process. Kept to be used in Reset() checking if
	// Chrome process hasn't restarted.
	proc *process.Process
	// kiosk is a reference to the Kiosk intstance.
	kiosk *kioskmode.Kiosk
	// lacros is a flag indicating whether fixture implementation suppose to run Lacros.
	lacros bool
	// signinTestExtensionManifestKey is the manifest key of the test extension used to interact with
	// Chrome in the sign in screen.
	signinTestExtensionManifestKey string
}

// KioskFixtData is returned by the fixture.
type KioskFixtData struct {
	// fakeDMS is an already running DMS server.
	fakeDMS *fakedms.FakeDMS
	// chrome is a connection to an already-started Chrome instance that loads policies from FakeDMS.
	chrome *chrome.Chrome
}

// Chrome implements the HasChrome interface.
func (f KioskFixtData) Chrome() *chrome.Chrome {
	if f.chrome == nil {
		panic("Chrome is called with nil chrome instance")
	}
	return f.chrome
}

// FakeDMS implements the HasFakeDMS interface.
func (f KioskFixtData) FakeDMS() *fakedms.FakeDMS {
	if f.fakeDMS == nil {
		panic("FakeDMS is called with nil fakeDMS instance")
	}
	return f.fakeDMS
}

func (k *kioskFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Parent is not a fakeDMSEnrolled fixture")
	}

	k.fdms = fdms
	k.signinTestExtensionManifestKey = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		k.signinTestExtensionManifestKey,
		kioskmode.AutoLaunch(k.autoLaunchKioskAppID),
		kioskmode.PublicAccountPolicies(k.autoLaunchKioskAppID, k.extraPublicAccountPolicies),
	)
	if err != nil {
		// Defer screenshot capture to make sure err is reported first.
		defer func(ctx context.Context) {
			if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), "kiosk_fixture.png")); err != nil {
				s.Error("Failed to take screenshot: ", err)
			}
		}(cleanupCtx)
		s.Fatal("Failed to create Chrome in kiosk mode: ", err)
	}
	// Make sure to clean up kiosk if an error occurs after this point.
	defer func(ctx context.Context) {
		if s.HasError() {
			if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), "kiosk_fixture.png")); err != nil {
				s.Error("Failed to take screenshot: ", err)
			}
			if err := kiosk.Close(ctx); err != nil {
				s.Error("Failed to close Kiosk: ", err)
			}
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if k.lacros {
		testConn, err := cr.TestAPIConn(ctx)
		_, err = lacrosproc.Root(ctx, testConn)
		if err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}

	proc, err := ashproc.Root()
	if err != nil {
		s.Fatal("Failed to get root Chrome PID: ", err)
	}

	chrome.Lock()
	k.cr = cr
	k.proc = proc
	k.kiosk = kiosk
	return &KioskFixtData{k.fdms, k.cr}
}

func (k *kioskFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()

	if k.cr == nil {
		s.Fatal("Chrome not yet started")
	}

	if err := k.kiosk.Close(ctx); err != nil {
		s.Error("Failed to close Kiosk: ", err)
	}

	k.cr = nil
}

func (k *kioskFixture) Reset(ctx context.Context) error {
	// Check the connection to Chrome.
	if err := k.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}

	// Check if the root chrome process is still running.
	if r, err := k.proc.IsRunning(); err != nil || !r {
		return errors.New("found root chrome process termination while running in Kiosk mode")
	}
	return nil
}

func (k *kioskFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}
func (k *kioskFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	tconn, err := k.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create TestAPI connection: ", err)
	}

	policies, err := policyutil.PoliciesFromDUT(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain policies from Chrome: ", err)
	}

	b, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		s.Fatal("Failed to marshal policies: ", err)
	}

	// Dump all policies as seen by Chrome to the tests OutDir.
	if err := ioutil.WriteFile(filepath.Join(s.OutDir(), "policies.json"), b, 0644); err != nil {
		s.Error("Failed to dump policies to file: ", err)
	}
}
