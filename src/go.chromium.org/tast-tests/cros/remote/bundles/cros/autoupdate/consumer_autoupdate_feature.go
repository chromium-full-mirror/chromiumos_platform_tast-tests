// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	ue "go.chromium.org/tast-tests/cros/common/updateengine"
	"go.chromium.org/tast-tests/cros/remote/updateutil"

	"go.chromium.org/tast/core/lsbrelease"
	"go.chromium.org/tast/core/testing"
)

const (
	preUpdateTimeout  = 1 * time.Minute
	postUpdateTimeout = 2 * time.Minute
)

type featuresTestParam struct {
	enabled bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ConsumerAutoupdateFeature,
		Desc: "Verifies that consumer auto update feature work as intended",
		Contacts: []string{
			"chromeos-core-services@google.com",
			"kimjae@chromium.org",
		},
		BugComponent: "b:908319",
		Attr: []string{
			"group:autoupdate",
			"group:release-health",
			"release-health_autoupdate",
		},
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		ServiceDeps: []string{
			"tast.cros.nebraska.Service",
			"tast.cros.autoupdate.UpdateService",
		},
		Timeout: preUpdateTimeout + updateutil.UpdateTimeout + postUpdateTimeout,
		Fixture: fixture.Autoupdate, // Autoupdate fixture cleans up and ensures the original image is restored.
		Params: []testing.Param{
			{
				Name: "enabled",
				Val: featuresTestParam{
					enabled: true,
				},
			},
			{
				Name: "disabled",
				Val: featuresTestParam{
					enabled: false,
				},
			},
		},
	})
}

func ConsumerAutoupdateFeature(ctx context.Context, s *testing.State) {
	// Limit the timeout for the preparation steps.
	preCtx, cancel := context.WithTimeout(ctx, preUpdateTimeout)
	defer cancel()

	// Builder path is used in selecting the update image.
	builderPath, err := updateutil.EntryFromLSBRelease(preCtx, s.DUT(), s.RPCHint(), lsbrelease.BuilderPath)
	if err != nil {
		s.Fatal("Failed to get builder path from lsb-release: ", err)
	}

	enabled := s.Param().(featuresTestParam).enabled
	var arg string
	var targetStatus ue.UpdateStatus
	if enabled {
		arg = "--enable_feature=" + string(ue.ConsumerAutoUpdate)
		targetStatus = ue.UpdateStatusUpdatedNeedReboot
	} else {
		arg = "--disable_feature=" + string(ue.ConsumerAutoUpdate)
		targetStatus = ue.UpdateStatusUpdatedButDeferred
	}
	if err := s.DUT().Conn().CommandContext(preCtx, "update_engine_client", arg).Run(); err != nil {
		s.Fatalf("Failed to toggle feature %s to %t: %v", ue.ConsumerAutoUpdate, enabled, err)
	}

	// Update the DUT.
	if err := updateutil.PeriodicUpdateFromGS(ctx, s.DUT(), s.OutDir(), s.RPCHint(), builderPath, string(targetStatus)); err != nil {
		s.Fatalf("Failed to update DUT to image for %q from GS: %v", builderPath, err)
	}

	// Limit the timeout for the post-update steps.
	postCtx, cancel := context.WithTimeout(ctx, postUpdateTimeout)
	defer cancel()
	if enabled {
		s.Log("Rebooting the DUT after the update")
		if err := s.DUT().Reboot(postCtx); err != nil {
			s.Fatal("Failed to reboot the DUT after update: ", err)
		}
	} else {
		s.Log("Applying the deferred update")
		if err := updateutil.ApplyDeferredUpdate(postCtx, s.DUT()); err != nil {
			s.Fatal("Failed to apply deferred update: ", err)
		}
	}
}
