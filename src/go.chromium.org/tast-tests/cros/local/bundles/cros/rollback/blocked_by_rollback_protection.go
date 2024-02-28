// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	nebraskapkg "go.chromium.org/tast-tests/cros/local/nebraska"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type rollbackBlockerTestParam struct {
	imageKernelAndFirmwareVersion [2]string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BlockedByRollbackProtection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that no rollback happens if firmware or kernel version of the rollback image are too low",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"mpolzer@google.com",
		},
		BugComponent: "b:1031231",
		SoftwareDeps: []string{"vpd"}, // Do not run on VM. Test needs NVRAM flash storage to work.
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      4 * time.Minute,
		Fixture:      fixture.ChromeUpdateEngineEnrolledLoggedIn,
		Params: []testing.Param{{
			Name: "kernel_rollback_protection",
			Val: &rollbackBlockerTestParam{
				imageKernelAndFirmwareVersion: [2]string{"0.0", "1.1"},
			},
		}, {
			Name: "firmware_rollback_protection",
			Val: &rollbackBlockerTestParam{
				imageKernelAndFirmwareVersion: [2]string{"1.1", "0.0"},
			},
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceRollbackToTargetVersion{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that unsupported devices do not roll back.
				// COM_FOUND_CUJ13_TASK5_WF1
				// {"blocker_type":"firmware-rollback-protection"}
				Value: "screenplay-ea371b35-e41e-4415-93d6-967fd63a461b",
			},
			{
				Key: "feature_id",
				// Configure "Roll back to target version" in Admin Console
				// policy and ensure that unsupported devices do not roll back.
				// COM_FOUND_CUJ13_TASK5_WF1
				// {"blocker_type":"kernel-rollback-protection"}
				Value: "screenplay-71dddef4-cc54-4690-9d3a-320e6627874f",
			},
		},
	})
}

// BlockedByRollbackProtection tests that no rollback happens if firmware or kernel (rollback protection) version of the rollback image are too low.
func BlockedByRollbackProtection(ctx context.Context, s *testing.State) {
	// Reserve one minute for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	const rollBackAndPreserveData = 3
	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, []policy.Policy{&policy.DeviceRollbackToTargetVersion{Val: rollBackAndPreserveData}}); err != nil {
		s.Fatal("Failed to set Rollback policy: ", err)
	}

	nebraska, err := nebraskapkg.New(ctx)
	if err != nil {
		s.Fatal("Failed to start Nebraska: ", err)
	}
	defer nebraska.Close(cleanupCtx)

	if err := nebraska.SetFakedMetadata(ctx); err != nil {
		s.Fatal("Failed to fake metadata for Nebraska: ", err)
	}

	if err := nebraska.SetIsRollback(ctx, true); err != nil {
		s.Fatal("Failed to configure Nebraska to server rollback: ", err)
	}

	param := s.Param().(*rollbackBlockerTestParam)
	if err := nebraska.SetRollbackPreventionImage(ctx, param.imageKernelAndFirmwareVersion); err != nil {
		s.Fatal("Failed to configure nebraska to send kernel and fw versions: ", err)
	}

	// No need to worry about the update not being blocked. We are not offering any payload. All update attempts will fail.
	if err := testexec.CommandContext(ctx,
		"update_engine_client",
		fmt.Sprintf("--omaha_url=http://localhost:%d/update", nebraska.Port),
		"--update",
	).Run(testexec.DumpLogOnError); err == nil {
		s.Fatal("Update succeeded but it should have failed with error kRollbackNotPossible")
	}

	out, err := testexec.CommandContext(ctx, "update_engine_client",
		"--last_attempt_error").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed check update check status: ", err)
	}

	if !strings.Contains(string(out), "kRollbackNotPossible") {
		s.Fatalf("Update was not blocked because of rollback protection, update engine output: %s", out)
	}
}
