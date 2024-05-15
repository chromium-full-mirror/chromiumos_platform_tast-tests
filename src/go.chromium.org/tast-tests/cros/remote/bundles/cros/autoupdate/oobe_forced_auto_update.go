// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	crui "go.chromium.org/tast-tests/cros/remote/cros/ui"
	oobeui "go.chromium.org/tast-tests/cros/remote/cros/ui/oobeui"
	"go.chromium.org/tast-tests/cros/remote/dut"
	"go.chromium.org/tast-tests/cros/remote/updateutil"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeForcedAutoUpdate,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that forced auto update is applied during OOBE",
		Contacts: []string{
			"cros-oobe@google.com",
			"dkuzmin@google.com", // Test owner
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
			"tast.cros.ui.AutomationService",
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.nebraska.Service",
		},
		Timeout: 8*time.Minute + updateutil.UpdateTimeout,
		Fixture: fixture.ForcedAutoupdate,
	})
}

func OobeForcedAutoUpdate(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	lsbContent := map[string]string{
		lsbrelease.Version:     "",
		lsbrelease.BuilderPath: "",
	}

	err := updateutil.FillFromLSBRelease(ctx, s.DUT(), s.RPCHint(), lsbContent)
	if err != nil {
		s.Fatal("Failed to get all the required information from lsb-release: ", err)
	}
	// Original image version to compare it with the version after the update.
	originalVersion := lsbContent[lsbrelease.Version]
	// Builder path is used in selecting the update image.
	builderPath := lsbContent[lsbrelease.BuilderPath]

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())

	// Create and set up a Nebraska client for an in-place update.
	nebraskaClient, _, err := updateutil.ConfigureNebraskaFromGS(ctx, cl.Conn, s.DUT(), s.OutDir(), builderPath)
	if err != nil {
		s.Fatal("Failed to set up nebraska for an in-place update: ", err)
	}

	// Configure Nebraska to serve a critical update.
	if _, err := nebraskaClient.SetCriticalUpdate(ctx, &nebraska.SetCriticalUpdateRequest{IsCritical: true}); err != nil {
		s.Fatal("Failed to set critical update for Nebraska: ", err)
	}

	// Retrieve the current root partition name to check if the update happens.
	oldRootPartition, err := dut.ReadCurrentRootPartitionName(ctx, s.DUT().Conn())
	if err != nil {
		s.Fatal("Failed to get current root partition: ", err)
	}

	crSvc := ui.NewChromeServiceClient(cl.Conn)
	defer func(ctx context.Context) {
		crSvc.Close(ctx, &empty.Empty{})
	}(cleanupCtx)

	crUISvc := ui.NewChromeUIServiceClient(cl.Conn)

	if _, err := crSvc.New(ctx, &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		KeepState:                    true,
		SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}); err != nil {
		s.Fatal("Failed to boot DUT to OOBE screen: ", err)
	}

	// Wait for WelcomeScreen to appear and OOBE to be ready.
	if _, err := crUISvc.WaitForWelcomeScreen(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to wait for OOBE is ready for testing: ", err)
	}

	// Click "Get started" to go to UpdateScreen (NetworkScreen should be skipped).
	uiautoSvc := ui.NewAutomationServiceClient(cl.Conn)
	if _, err := uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: oobeui.GetStartedButtonFinder}); err != nil {
		s.Fatal("Failed to click the Get started button: ", err)
	}

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.RestartingToApplyUpdateName, updateutil.UpdateTimeout); err != nil {
		s.Log("Failed to find node, but still try to wait for reboot: ", err)
	}

	// Await the reboot and ensure root partition changed.
	if err := updateutil.WaitForUpdateReboot(ctx, s.DUT(), oldRootPartition); err != nil {
		s.Fatal("Failed to await reboot into the new image: ", err)
	}

	// Check the image version.
	version, err := updateutil.ImageVersion(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to read DUT image version after the update: ", err)
	}
	s.Logf("The DUT image version after the update is %s", version)
	if version != originalVersion {
		s.Errorf("Image version changed after the update; got %s, want %s", version, originalVersion)
	}
}
