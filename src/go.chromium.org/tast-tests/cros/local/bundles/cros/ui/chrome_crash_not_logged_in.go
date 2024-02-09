// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/chromecrash"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// chromeCrashNotLoggedInParams contains the test parameters which are different between the various tests.
type chromeCrashNotLoggedInParams struct {
	ptype   chromecrash.ProcessType
	handler chromecrash.CrashHandler
	consent crash.ConsentType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeCrashNotLoggedIn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome writes crash dumps while not logged in",
		Contacts:     []string{"chromeos-data-eng@google.com", "iby@chromium.org"},
		BugComponent: "b:1032705",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			Name: "browser_breakpad",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.Browser,
				handler: chromecrash.Breakpad,
				consent: crash.RealConsent,
			},
			ExtraSoftwareDeps: []string{"breakpad", "metrics_consent"},
			// This test performs 2 logins.
			Timeout: 2*chrome.LoginTimeout + time.Minute,
		}, {
			Name: "browser_breakpad_mock_consent",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.Browser,
				handler: chromecrash.Breakpad,
				consent: crash.MockConsent,
			},
			ExtraSoftwareDeps: []string{"breakpad"},
		}, {
			Name: "browser_crashpad",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.Browser,
				handler: chromecrash.Crashpad,
				consent: crash.RealConsent,
			},
			ExtraSoftwareDeps: []string{"crashpad", "metrics_consent"},
			// This test performs 2 logins.
			Timeout: 2*chrome.LoginTimeout + time.Minute,
		}, {
			Name: "browser_crashpad_mock_consent",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.Browser,
				handler: chromecrash.Crashpad,
				consent: crash.MockConsent,
			},
			ExtraSoftwareDeps: []string{"crashpad"},
			ExtraAttr:         []string{"group:hw_agnostic"},
		}, {
			Name: "gpu_process_breakpad",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.GPUProcess,
				handler: chromecrash.Breakpad,
				consent: crash.RealConsent,
			},
			ExtraSoftwareDeps: []string{"breakpad", "metrics_consent"},
			// This test performs 2 logins.
			Timeout: 2*chrome.LoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_breakpad_mock_consent",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.GPUProcess,
				handler: chromecrash.Breakpad,
				consent: crash.MockConsent,
			},
			ExtraSoftwareDeps: []string{"breakpad"},
		}, {
			Name: "gpu_process_crashpad",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.GPUProcess,
				handler: chromecrash.Crashpad,
				consent: crash.RealConsent,
			},
			ExtraSoftwareDeps: []string{"crashpad", "metrics_consent"},
			// This test performs 2 logins.
			Timeout: 2*chrome.LoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_crashpad_mock_consent",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.GPUProcess,
				handler: chromecrash.Crashpad,
				consent: crash.MockConsent,
			},
			ExtraSoftwareDeps: []string{"crashpad"},
			ExtraAttr:         []string{"group:hw_agnostic"},
		}, {
			Name: "broker_breakpad_mock_consent",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.Broker,
				handler: chromecrash.Breakpad,
				consent: crash.MockConsent,
			},
			// If the gpu process is not sandboxed, it will not create a broker.
			ExtraSoftwareDeps: []string{"breakpad", "gpu_sandboxing"},
		}, {
			Name: "broker_crashpad_mock_consent",
			Val: chromeCrashNotLoggedInParams{
				ptype:   chromecrash.Broker,
				handler: chromecrash.Crashpad,
				consent: crash.MockConsent,
			},
			// If the gpu process is not sandboxed, it will not create a broker.
			ExtraSoftwareDeps: []string{"crashpad", "gpu_sandboxing"},
		}},
	})
}

func ChromeCrashNotLoggedIn(ctx context.Context, s *testing.State) {
	params := s.Param().(chromeCrashNotLoggedInParams)
	ct, err := chromecrash.NewCrashTester(ctx, params.ptype, browser.TypeAsh, chromecrash.MetaFile)
	if err != nil {
		s.Fatal("NewCrashTester failed: ", err)
	}
	defer ct.Close()

	extraArgs := chromecrash.GetExtraArgs(params.handler, params.consent)
	chromeOpts := []chrome.Option{chrome.NoLogin(), chrome.CrashNormalMode(), chrome.ExtraArgs(extraArgs...)}

	if params.consent == crash.RealConsent {
		// We need to be logged in to set up consent, but then log out for the actual test.
		if err := func() error {
			cr, err := chrome.New(ctx, chrome.CrashNormalMode(), chrome.ExtraArgs(extraArgs...))
			if err != nil {
				return errors.Wrap(err, "chrome startup failed")
			}
			defer cr.Close(ctx)
			if err := crash.SetUpCrashTest(ctx, crash.WithConsent(cr)); err != nil {
				return errors.Wrap(err, "SetUpCrashTest failed")
			}
			return nil
		}(); err != nil {
			s.Fatal("Setting up crash test failed: ", err)
		}
		// Need to KeepState to avoid erasing the consent we just set up
		chromeOpts = append(chromeOpts, chrome.KeepState())
	} else {
		if err = crash.SetUpCrashTest(ctx, crash.WithMockConsent()); err != nil {
			s.Fatal("Setting up crash test failed: ", err)
		}
	}
	defer crash.TearDownCrashTest(ctx)

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Chrome startup failed: ", err)
	}
	defer cr.Close(ctx)
	if err := ct.AssociateWithChrome(ctx, cr); err != nil {
		s.Fatal("Failed to associate chrome with the crash tester: ", err)
	}

	files, err := ct.KillAndGetCrashFiles(ctx)
	if err != nil {
		s.Fatalf("Couldn't kill Chrome %s process or get files: %v", params.ptype, err)
	}

	// Not-logged-in Chrome crashes get logged to /home/chronos/crash, not the
	// default /var/spool/crash, since it still runs as user "chronos".
	if err = chromecrash.FindCrashFilesIn(crash.LocalCrashDir, files); err != nil {
		s.Errorf("Crash files weren't written to /home/chronos/crash after crashing %s process: %v", params.ptype, err)
	}
}
