// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelWarning,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify kernel warnings are logged as expected",
		Contacts:     []string{"chromeos-data-eng@google.com", "iby@chromium.org"},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline"},
		// TODO(b/201790026): The lkdtm resides on the debugfs,
		// which is not accessible when integrity mode is
		// enabled.
		//
		// We either need to refactor the test not to use lkdtm
		// to trigger crashes or we need to modify the kernel to
		// allow access to the required path. Skip on reven for
		// now, since reven uses integrity mode.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("reven")),
		Params: []testing.Param{{
			Name:              "real_consent",
			ExtraSoftwareDeps: []string{"chrome", "metrics_consent"},
			ExtraAttr:         []string{"informational"},
			Val:               crash.RealConsent,
			Fixture:           crash.LoggedInRealConsent,
		}, {
			Name:    "mock_consent",
			Val:     crash.MockConsent,
			Fixture: crash.MockConsentFixture,
		}, {
			Name:              "real_consent_per_user_on",
			ExtraSoftwareDeps: []string{"chrome", "metrics_consent"},
			ExtraAttr:         []string{"informational"},
			// No fixture because we must manually log in and out to chrome
			// on two accounts.
			Val: crash.RealConsentPerUserOn,
			// This test performs 2 logins.
			Timeout: 2*chrome.LoginTimeout + time.Minute,
		}, {
			Name:              "real_consent_per_user_off",
			ExtraSoftwareDeps: []string{"chrome", "metrics_consent"},
			ExtraAttr:         []string{"informational"},
			// No fixture because we must manually log in and out to chrome
			// on two accounts.
			Val: crash.RealConsentPerUserOff,
			// This test performs 2 logins.
			Timeout: 2*chrome.LoginTimeout + time.Minute,
		}},
	})
}

func KernelWarning(ctx context.Context, s *testing.State) {
	consentType := s.Param().(crash.ConsentType)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	usePerUser := consentType == crash.RealConsentPerUserOn || consentType == crash.RealConsentPerUserOff
	if usePerUser {
		// First, create and log out of chrome, as a primary user (device owner).
		if err := func() error {
			cr, err := chrome.New(ctx, chrome.ExtraArgs(crash.ChromeVerboseConsentFlags))
			if err != nil {
				return errors.Wrap(err, "chrome startup failed")
			}
			defer cr.Close(cleanupCtx)
			if err := crash.SetUpCrashTest(ctx, crash.WithConsent(cr)); err != nil {
				return errors.Wrap(err, "SetUpCrashTest failed")
			}
			return nil
		}(); err != nil {
			s.Fatal("Setting up crash test failed: ", err)
		}
	} else {
		if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes("kernel_warning")); err != nil {
			s.Fatal("SetUpCrashTest failed: ", err)
		}
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	if usePerUser {
		cr, err := chrome.New(ctx,
			// Avoid erasing consent we just enabled, and in particular do *not* take ownership.
			chrome.KeepState(),
			chrome.FakeLogin(chrome.Creds{User: "additional-user1@gmail.com", Pass: "password"}))
		if err != nil {
			s.Fatal("Chrome startup failed: ", err)
		}
		defer cr.Close(cleanupCtx)
		if err := crash.CreatePerUserConsent(ctx, consentType == crash.RealConsentPerUserOn); err != nil {
			s.Fatal("Failed to create per-user consent: ", err)
		}
		defer func() {
			if err := crash.RemovePerUserConsent(cleanupCtx); err != nil {
				s.Error("Failed to clean up per-user consent: ", err)
			}
		}()
	}

	if err := crash.RestartAnomalyDetectorWithSendAll(ctx, true); err != nil {
		s.Fatal("Failed to restart anomaly detector: ", err)
	}
	// Restart anomaly detector to clear its --testonly-send-all flag at the end of execution.
	defer crash.RestartAnomalyDetector(cleanupCtx)

	s.Log("Inducing artificial warning")
	const lkdtm = "/sys/kernel/debug/provoke-crash/DIRECT"
	if err := ioutil.WriteFile(lkdtm, []byte("WARNING"), 0); err != nil {
		s.Fatal("Failed to induce warning in lkdtm: ", err)
	}

	s.Log("Waiting for files")
	const (
		funcName = `[a-zA-Z0-9_]*(?:lkdtm|direct_entry)[a-zA-Z0-9_]*`
		baseName = `kernel_warning_` + funcName + `\.\d{8}\.\d{6}\.\d+\.0`
		metaName = baseName + `\.meta`
	)
	expectedRegexes := []string{baseName + `\.kcrash`,
		baseName + `\.log\.gz`,
		metaName}
	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	if consentType == crash.MockConsent {
		// We might not be logged in, so also allow system crash dir.
		crashDirs = append(crashDirs, crash.SystemCrashDir)
	}
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, expectedRegexes)
	defer func() {
		if err := crash.RemoveAllFiles(cleanupCtx, files); err != nil {
			s.Log("Couldn't clean up files: ", err)
		}
	}()
	if consentType == crash.RealConsentPerUserOff {
		// In the "RealConsentPerUserOff" case, we expect a non-nil `err` value, and we expect
		// there to be *no* meta files returned, so we should not move to checking the meta files
		// below. If there are any, it's an error.
		if err == nil {
			if err := crash.MoveFilesToOut(ctx, s.OutDir(), files[metaName]...); err != nil {
				s.Error("Failed to save unexpected crash files: ", err)
			}
			s.Fatal("Found crash files but didn't expect to")
		}
		return
	}
	// For the other consentTypes, expect to find files.
	if err != nil {
		s.Fatal("Couldn't find expected files: ", err)
	}

	if len(files[metaName]) == 1 {
		metaFile := files[metaName][0]
		metaFileParametersToTest := map[string]string{
			"in_progress_integration_test": "crash.KernelWarning",
			"weight":                       "10",
			"client_computed_severity":     "WARNING",
			"client_computed_product":      "Platform",
		}
		if contents, err := ioutil.ReadFile(metaFile); err != nil {
			s.Errorf("Couldn't read meta file %s contents: %v", metaFile, err)
		} else {
			missing := false
			for key, value := range metaFileParametersToTest {
				if !strings.Contains(string(contents), ("upload_var_" + key + "=" + value)) {
					s.Error(".meta file did not contain expected " + key)
					missing = true
				}
			}
			if missing {
				if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
					s.Error("Failed to save the meta file: ", err)
				}
			}
		}
	} else {
		s.Errorf("Unexpectedly found multiple meta files: %q", files[metaName])
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), files[metaName]...); err != nil {
			s.Error("Failed to save additional meta files: ", err)
		}
	}
}
