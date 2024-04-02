// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// iterations is the number of times to try setting up and setting consent.
	iterations = 5
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Consent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify consent can be set and unset",
		Contacts:     []string{"chromeos-data-eng@google.com", "iby@chromium.org"},
		BugComponent: "b:1032705", // ChromeOS > Data > Engineering > Crash Reporting
		Attr:         []string{"group:mainline", "informational"},
		// tpm1 is not common and requires a reboot for clearing ownership, which would make the test more complex
		SoftwareDeps: []string{"chrome", "metrics_consent", "tpm2"},
		Timeout:      iterations * chrome.LoginTimeout,
	})
}

func setUpTearDown(ctx, cleanupCtx context.Context) error {
	// Create a chrome instance.
	cr, err := chrome.New(ctx, chrome.ExtraArgs(crash.ChromeVerboseConsentFlags))
	if err != nil {
		return errors.Wrap(err, "failed to create chrome")
	}
	// Close chrome with cleanupCtx so that if the test times out it still cleans up state.
	defer cr.Close(cleanupCtx)

	// Set up, turn on consent
	if err := crash.SetUpCrashTest(ctx, crash.WithConsent(cr)); err != nil {
		return errors.Wrap(err, "SetUpCrashTest failed")
	}
	// Tear down with cleanupCtx so that if the test times out it still cleans up state.
	defer crash.TearDownCrashTest(cleanupCtx)

	// Turn off consent
	if err := crash.SetConsent(ctx, cr, false); err != nil {
		return errors.Wrap(err, "failed to set consent to false")
	}

	return nil
}

// Consent stress-tests the consent/setup logic for crash tests by looping around:
// 1. Creating chrome / taking ownership
// 2. Setting consent
// 3. Clearing consent.
// This test should help us to characterize and debug consent flakiness, as it
// does *nothing* but the basics required to set consent.
func Consent(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	cmdRunner := hwsec.NewCmdRunner()
	helper, err := hwsec.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	defer func(ctx context.Context) {
		if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
			s.Error("Failed to clear ownership: ", err)
		}
	}(cleanupCtx)

	for i := 0; i < iterations; i++ {
		// Clear ownership.
		if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
			s.Fatal("Failed to clear ownership: ", err)
		}

		if err := setUpTearDown(ctx, cleanupCtx); err != nil {
			s.Fatalf("Failed setup on iteration %d: %v", i, err)
		}
	}
}
