// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"chromiumos/system_api/dlcservice_proto"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasDLCManager,
		Desc:         "Check if CRAS can successfully install DLC packages",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "hunghsienchen@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome", "dlc"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("amd64-generic")),
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// CrasDLCManager checks if CRAS can successfully install DLC packages
func CrasDLCManager(ctx context.Context, s *testing.State) {
	dlcIDs := []string{"nc-ap-dlc", "sr-bt-dlc"}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	chrome, err := chrome.New(
		ctx,
		// org.chromium.ChromeFeaturesService does not need login to work.
		// Don't login to speed up the test.
		chrome.NoLogin(),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer chrome.Close(cleanupCtx)

	s.Log("Stopping CRAS")
	if err := upstart.StopJob(ctx, "cras"); err != nil {
		s.Fatal("Failed to stop CRAS: ", err)
	}
	defer upstart.EnsureJobRunning(cleanupCtx, "cras")

	// CRAS might already started DLC installation before being stopped
	s.Log("Checking DLC states")
	if err := waitUntilAllDLCInstallTerminate(ctx, dlcIDs); err != nil {
		s.Fatal("Some installation took too long, or failed to get DLC state: ", err)
	}

	s.Log("Uninstalling the DLCs")
	for _, dlcID := range dlcIDs {
		// Uninstall is no-op when the DLC is not installed
		if err := dlc.Uninstall(ctx, dlcID); err != nil {
			s.Fatalf("Failed to uninstall DLC %q: %v", dlcID, err)
		}
	}

	// Check if the DLCs are uninstalled
	for _, dlcID := range dlcIDs {
		dlcState, err := dlc.GetDlcState(ctx, dlcID)
		if err != nil {
			s.Fatalf("Failed to get state of DLC %q", dlcID)
		}
		dlcStateState := dlcservice_proto.DlcState_State(dlcState.State)
		if dlcStateState != dlcservice_proto.DlcState_NOT_INSTALLED {
			s.Fatalf("Failed to uninstall DLC %q, state: %q", dlcID, dlcservice_proto.DlcState_State_name[int32(dlcStateState)])
		}
	}

	s.Log("Starting CRAS (which should trigger DLC install)")
	if err := upstart.EnsureJobRunning(ctx, "cras"); err != nil {
		s.Fatal("Failed to start CRAS: ", err)
	}

	// Wait until all DLCs are installed
	s.Log("Checking DLC state")
	if err := waitUntilAllDLCAreInstalled(ctx, dlcIDs); err != nil {
		for _, dlcID := range dlcIDs {
			dlcState, _ := dlc.GetDlcState(ctx, dlcID)
			dlcStateState := dlcservice_proto.DlcState_State(dlcState.State)
			if dlcStateState != dlcservice_proto.DlcState_INSTALLED {
				s.Logf("DLC %q is not installed", dlcID)
			}
		}
		s.Fatal("Failed to install some DLCs: ", err)
	}
}

// waitUntilAllDLCInstallTerminate polls the state of each `dlcIDs` until all installations are done, either success or failed.
func waitUntilAllDLCInstallTerminate(ctx context.Context, dlcIDs []string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		for _, dlcID := range dlcIDs {
			dlcState, err := dlc.GetDlcState(ctx, dlcID)
			if err != nil {
				return testing.PollBreak(errors.Wrapf(err, "failed to get state of DLC %q", dlcID))
			}
			dlcStateState := dlcservice_proto.DlcState_State(dlcState.State)
			if dlcStateState == dlcservice_proto.DlcState_INSTALLING {
				return errors.Errorf("DLC %q is still installing", dlcID)
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  1 * time.Minute,
		Interval: 1 * time.Second,
	})
}

// waitUntilAllDLCAreInstalled polls the state of each `dlcIDs` until all are installed.
func waitUntilAllDLCAreInstalled(ctx context.Context, dlcIDs []string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		for _, dlcID := range dlcIDs {
			dlcState, err := dlc.GetDlcState(ctx, dlcID)
			if err != nil {
				return testing.PollBreak(errors.Wrapf(err, "failed to get state of DLC %q", dlcID))
			}
			dlcStateState := dlcservice_proto.DlcState_State(dlcState.State)
			if dlcStateState != dlcservice_proto.DlcState_INSTALLED {
				return errors.Errorf("DLC %q is not installed", dlcID)
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  3 * time.Minute,
		Interval: 3 * time.Second,
	})
}
