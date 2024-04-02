// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/chromecrash"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// chromeCrashLoggedInParams contains the test parameters which are different between the various tests.
type chromeCrashLoggedInParams struct {
	ptype                chromecrash.ProcessType
	handler              chromecrash.CrashHandler
	browserType          browser.Type
	consent              crash.ConsentType
	fieldTrialConfigMode chrome.FieldTrialConfigMode
	restartChrome        bool
}

func init() {
	// Note: There are no Lacros variants for breakpad tests because we are close
	// enough to deprecating breakpad that there's no value in adding them.
	testing.AddTest(&testing.Test{
		Func:         ChromeCrashLoggedIn,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that Chrome writes crash dumps while logged in",
		Contacts:     []string{"chromeos-data-eng@google.com", "iby@chromium.org"},
		BugComponent: "b:1032705",
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "browser_breakpad",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Breakpad,
				browserType:          browser.TypeAsh,
				consent:              crash.RealConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"breakpad", "metrics_consent"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_breakpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Breakpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"breakpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_crashpad",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.RealConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"crashpad", "metrics_consent"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_lacros_crashpad",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.RealConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"crashpad", "metrics_consent", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_crashpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_crashpad_mock_consent_field_trials_on",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigEnable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_crashpad_mock_consent_field_trials_off",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDisable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_lacros_crashpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_lacros_crashpad_mock_consent_field_trials_on",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigEnable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "browser_lacros_crashpad_mock_consent_field_trials_off",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Browser,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDisable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_breakpad",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Breakpad,
				consent:              crash.RealConsent,
				browserType:          browser.TypeAsh,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
				// Breakpad only adds a signal handler to a process if it has consent
				// at the time the process starts up. There's a special hook in the
				// browser process to initialize Breakpad if consent is given after
				// startup
				// (https://chromium-review.googlesource.com/c/chromium/src/+/2145676);
				// however, this only works for the browser process. For the GPU
				// process, we need to set consent and then restart Chrome. The
				// mock_consent variants use a command-line argument which forces the
				// 'install a signal handler' behavior, so we only need this for the
				// real consent variant.
				restartChrome: true,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"breakpad", "metrics_consent"},
			// This test performs 2 logins.
			Timeout: 2*chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_breakpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Breakpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"breakpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_crashpad",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.RealConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"crashpad", "metrics_consent"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_lacros_crashpad",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.RealConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline"},
			ExtraSoftwareDeps: []string{"crashpad", "metrics_consent", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_crashpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_crashpad_mock_consent_field_trial_on",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigEnable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_crashpad_mock_consent_field_trial_off",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDisable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_lacros_crashpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_lacros_crashpad_mock_consent_field_trial_on",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigEnable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "gpu_process_lacros_crashpad_mock_consent_field_trial_off",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.GPUProcess,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDisable,
			},
			ExtraAttr:         []string{"group:mainline", "group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"crashpad", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "broker_breakpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Broker,
				handler:              chromecrash.Breakpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr: []string{"group:mainline"},
			// If the gpu process is not sandboxed, it will not create a broker.
			ExtraSoftwareDeps: []string{"breakpad", "gpu_sandboxing"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "broker_crashpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Broker,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeAsh,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr: []string{"group:mainline"},
			// If the gpu process is not sandboxed, it will not create a broker.
			ExtraSoftwareDeps: []string{"crashpad", "gpu_sandboxing"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}, {
			Name: "broker_lacros_crashpad_mock_consent",
			Val: chromeCrashLoggedInParams{
				ptype:                chromecrash.Broker,
				handler:              chromecrash.Crashpad,
				browserType:          browser.TypeLacros,
				consent:              crash.MockConsent,
				fieldTrialConfigMode: chrome.FieldTrialConfigDefault,
			},
			ExtraAttr: []string{"group:mainline"},
			// If the gpu process is not sandboxed, it will not create a broker.
			ExtraSoftwareDeps: []string{"crashpad", "gpu_sandboxing", "lacros"},
			Timeout:           chrome.MinLoginTimeout + time.Minute,
		}},
	})
}

func ChromeCrashLoggedIn(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	params := s.Param().(chromeCrashLoggedInParams)
	ct, err := chromecrash.NewCrashTester(ctx, params.ptype, params.browserType, chromecrash.MetaFile)
	if err != nil {
		s.Fatal("NewCrashTester failed: ", err)
	}
	defer ct.Close()

	extraArgs := chromecrash.GetExtraArgs(params.handler, params.consent)
	chromeOpts := []chrome.Option{chrome.CrashNormalMode(), chrome.ExtraArgs(extraArgs...)}
	if params.fieldTrialConfigMode != chrome.FieldTrialConfigDefault {
		chromeOpts = append(chromeOpts, chrome.FieldTrialConfig(params.fieldTrialConfigMode))
	}
	// In theory it would nice to rewrite this to use fixtures "correctly" but
	// there's significant engineering work for that (b/292145636).
	cr, _, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, params.browserType,
		lacrosfixt.NewConfig(), chromeOpts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer func() {
		if cr != nil {
			closeBrowser(cleanupCtx)
			cr.Close(cleanupCtx)
		}
	}()

	opt := crash.WithMockConsent()
	if params.consent == crash.RealConsent {
		opt = crash.WithConsent(cr)
	}
	if err := crash.SetUpCrashTest(ctx, opt); err != nil {
		s.Fatal("SetUpCrashTest failed: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	if params.restartChrome {
		closeBrowser(ctx)
		cr.Close(ctx)
		// Need to KeepState to avoid erasing the consent we just set up.
		restartOpts := append(chromeOpts, chrome.KeepState())
		cr, _, closeBrowser, err = browserfixt.SetUpWithNewChrome(ctx, params.browserType,
			lacrosfixt.NewConfig(), restartOpts...)
		if err != nil {
			cr = nil
			s.Fatal("Chrome login failed: ", err)
		}
	}

	if err := ct.AssociateWithChrome(ctx, cr); err != nil {
		s.Fatal("Failed to associate chrome with the crash tester: ", err)
	}

	files, err := ct.KillAndGetCrashFiles(ctx)
	if err != nil {
		s.Fatalf("Couldn't kill Chrome %s process or get files: %v", params.ptype, err)
	}

	if err = chromecrash.FindCrashFilesIn(chromecrash.CryptohomePattern, files); err != nil {
		s.Errorf("Crash files weren't written to cryptohome after crashing the %s process: %v", params.ptype, err)
		// So we've seen weird problems where the meta files get created but by the
		// time 'newFiles, err := crash.GetCrashes(dirs...)' runs inside
		// KillAndGetCrashFiles, the meta files aren't found. Add more debugging
		// output to diagnose. crbug.com/1080365
		args := []string{"-lia", "/home/chronos", "/home/user"}
		if paths, err := filepath.Glob(chromecrash.CryptohomePattern); err != nil {
			s.Log("Error getting cryptohomes from ", chromecrash.CryptohomePattern, ": ", err)
		} else {
			for _, path := range paths {
				args = append(args, path, filepath.Join(path, "crash"))
			}
		}
		if paths, err := filepath.Glob("/home/user/*"); err != nil {
			s.Log("Error getting cryptohomes from /home/user/*: ", err)
		} else {
			for _, path := range paths {
				args = append(args, path, filepath.Join(path, "crash"))
			}
		}

		cmd := testexec.CommandContext(ctx, "/bin/ls", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			s.Logf("ls of %v failed: %v", args, err)
		}
		outfile := filepath.Join(s.OutDir(), "ls_output.txt")
		if err := ioutil.WriteFile(outfile, out, 0644); err != nil {
			s.Logf("Storing ls output %v to %v failed: %v", out, outfile, err)
		}
	}
}
