// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SenderNoConsent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that crashes are not uploaded without consent",
		Contacts: []string{
			"cros-telemetry@google.com",
			"mutexlox@chromium.org",
			"iby@chromium.org",
			"nya@chromium.org", // ported to Tast
		},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline"},
		// We only care about crash_sender on internal builds.
		SoftwareDeps: []string{"chrome", "cros_internal", "metrics_consent"},
		Params: []testing.Param{{
			Name:      "fieldtrial_testing_config_on",
			ExtraAttr: []string{"informational", "group:criticalstaging"},
			Fixture:   fixture.ChromeLoggedInWithFieldTrialConfigEnableAndVerboseConsent,
		}, {
			Name:    "fieldtrial_testing_config_off",
			Fixture: fixture.ChromeLoggedInWithFieldTrialConfigDisableAndVerboseConsent,
		}},
	})
}

func SenderNoConsent(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
	if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes(crash.FilterInIgnoreAllCrashes), crash.WithConsent(cr)); err != nil {
		s.Fatal("Setup failed: ", err)
	}
	defer crash.TearDownCrashTest(ctx)

	// Revoke the consent.
	if err := crash.SetConsent(ctx, cr, false); err != nil {
		s.Fatal("Failed to revoke consent: ", err)
	}

	const basename = "some_program.1.2.3.4"
	if _, err := crash.AddFakeMinidumpCrash(ctx, basename); err != nil {
		s.Fatal("Failed to add a fake minidump crash: ", err)
	}

	got, err := crash.RunSender(ctx)
	if err != nil {
		s.Fatal("Failed to run crash_sender: ", err)
	}
	var want []*crash.SendResult
	if diff := cmp.Diff(got, want); diff != "" {
		s.Log("Results mismatch (-got +want): ", diff)
		s.Errorf("crash_sender sent unexpected %d results; see logs for diff", len(got))
	}
}
