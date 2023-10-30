// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	empb "chromiumos/policy/chromium/policy/enterprise_management_proto"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceSetVolume,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks the behavior of sending device set volume remote command",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"mohamedaomar@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + 30*time.Second,
		Params: []testing.Param{
			{
				Name: "ash",
				Val:  kioskmode.TestData{IsLacros: false},
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: kioskmode.TestData{
					IsLacros: true,
					Policies: []policy.Policy{
						&policy.LacrosAvailability{Val: "lacros_only"},
					},
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Test remote volume level command.
				Value: "screenplay-6ba34335-2f1f-4f78-a115-9149348a59fe",
			},
		},
	})
}

func DeviceSetVolume(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(kioskmode.TestData)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		kioskmode.PublicAccountPolicies(kioskmode.WebKioskAccountID, param.Policies),
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	// Connect to Test API to use it for refreshing remote commands.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "kiosk_with_FloatingAccessibilityMenuEnabled")

	// Set up loopback devices as fallback.
	unload, err := audio.LoadAloop(ctx)
	if err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to load ALSA loopback module: ", err)
	}

	defer func(ctx context.Context) {
		// Wait for no stream before unloading aloop as unloading while there is a stream
		// will cause the stream in ARC to be in an invalid state.
		if err := crastestclient.WaitForNoStream(ctx, 5*time.Second); err != nil {
			s.Error("Wait for no stream error: ", err)
		}
		unload(ctx)
	}(cleanupCtx)

	// NewVolumeHelper setup also checks that there are existing audio devices to
	// adjust volume for.
	vh, err := audio.NewVolumeHelper(ctx)
	if err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to create audio volume helper: ", err)
	}

	const targetVolume int = 55
	resp, err := fdms.SendRemoteCommand(ctx, &empb.SendRemoteCommandRequest{
		RemoteCommand: &empb.RemoteCommand{
			Payload: &[]string{fmt.Sprintf(`{"volume": %d}`, targetVolume)}[0],
			Type:    &[]empb.RemoteCommand_Type{empb.RemoteCommand_DEVICE_SET_VOLUME}[0],
		},
	})
	if err != nil {
		s.Fatal("Couldn't send the remote command request: ", err)
	}

	// Maunally forcing Chrome to fetch remote commands.
	if err := policyutil.RefreshRemoteCommands(ctx, tconn); err != nil {
		s.Fatal("Failed to refresh remote commands: ", err)
	}

	result, err := fdms.WaitRemoteCommandResult(ctx, resp.CommandId)
	if err != nil {
		s.Fatal("Couldn't wait for remote command result: ", err)
	}

	if *result.Result.Result != empb.RemoteCommandResult_RESULT_SUCCESS {
		s.Error("The remote command wasn't executed successfully; found result: ", *result.Result.Result)
	}

	// Get current volume.
	curVolume, err := vh.GetVolume(ctx)
	if err != nil {
		s.Fatal("Failed to get output volume: ", err)
	}

	if curVolume != targetVolume {
		s.Errorf("Unexpected Volume set: got %d; want %d", curVolume, targetVolume)
	}
}
