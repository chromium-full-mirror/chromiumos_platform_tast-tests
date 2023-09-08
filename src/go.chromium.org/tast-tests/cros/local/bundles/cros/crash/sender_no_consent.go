// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"

	"github.com/google/go-cmp/cmp"

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
			Name:    "fieldtrial_testing_config_on",
			Fixture: crash.LoggedInNoConsentFieldTrialConfigEnable,
		}, {
			Name:    "fieldtrial_testing_config_off",
			Fixture: crash.LoggedInNoConsentFieldTrialConfigDisable,
		}},
	})
}

func SenderNoConsent(ctx context.Context, s *testing.State) {
	if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes(crash.FilterInIgnoreAllCrashes)); err != nil {
		s.Fatal("Setup failed: ", err)
	}
	defer crash.TearDownCrashTest(ctx)

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
