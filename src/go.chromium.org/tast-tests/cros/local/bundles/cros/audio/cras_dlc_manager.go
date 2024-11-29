// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"go.chromium.org/chromiumos/system_api/dlcservice_proto"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasDLCManager,
		Desc:         "Check if CRAS can successfully install DLC packages",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "hunghsienchen@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline"},
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome", "dlc", "cros_internal"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("amd64-generic")),
	})
}

func getDlcsToCheck(ctx context.Context, s *testing.State) []string {
	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Failed to restart CRAS: ", err)
	}

	dlcIDs := []string{"nc-ap-dlc"} // nc-ap-dlc is always installed.

	srBtSupported, err := cras.IsHfpMicSrSupported(ctx)
	if err != nil {
		s.Fatal("Failed when calling IsHfpMicSrSupported: ", err)
	}
	if srBtSupported {
		dlcIDs = append(dlcIDs, "sr-bt-dlc")
	}

	styleTransferSupported, err := cras.IsStyleTransferSupported(ctx)
	if err != nil {
		s.Fatal("Failed when calling IsStyleTransferSupported: ", err)
	}
	if styleTransferSupported {
		dlcIDs = append(dlcIDs, "nc-ap-dlc")
	}
	return dlcIDs
}

// CrasDLCManager checks if CRAS can successfully install DLC packages
func CrasDLCManager(ctx context.Context, s *testing.State) {
	dlcIDs := getDlcsToCheck(ctx, s)
	s.Logf("DLCs to check: %q", dlcIDs)

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

	// CRAS might already started DLC installation before being stopped.
	// Restart dlcservice to stop the installations.
	s.Logf("Restarting ui, update-engine, and %s", dlc.JobName)
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Log("Failed to restart ui: ", err)
	}
	if err := upstart.RestartJob(ctx, "update-engine"); err != nil {
		s.Log("Failed to restart update-engine: ", err)
	}
	if err := upstart.RestartJobAndWaitForDbusService(ctx, dlc.JobName, dlc.ServiceName); err != nil {
		s.Logf("Failed to restart %s and wait for %s: %v", dlc.JobName, dlc.ServiceName, err)
	}

	s.Log("Uninstalling the DLCs")
	uninstalled := make([]bool, len(dlcIDs))
	testing.Poll(ctx, func(ctx context.Context) error {
		for i, dlcID := range dlcIDs {
			if uninstalled[i] {
				continue
			}
			// Uninstall is no-op when the DLC is not installed
			if err := dlc.Uninstall(ctx, dlcID); err != nil {
				errors.Wrapf(err, "failed to uninstall DLC %q", dlcID)
			} else {
				uninstalled[i] = true
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  3 * time.Minute,
		Interval: 3 * time.Second,
	})

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
	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Failed to start CRAS: ", err)
	}

	s.Log("Waiting for audio_effects_ready in S2")
	if err := cras.WaitForAudioEffectsReady(ctx); err != nil {
		s.Fatal("cras.WaitForAudioEffectsReady: ", err)
	}

	s.Log("Check DLCs")
	for _, dlcID := range dlcIDs {
		dlcState, _ := dlc.GetDlcState(ctx, dlcID)
		dlcStateState := dlcservice_proto.DlcState_State(dlcState.State)
		if dlcStateState != dlcservice_proto.DlcState_INSTALLED {
			s.Errorf("DLC %q is not installed", dlcID)
		}
	}
}
