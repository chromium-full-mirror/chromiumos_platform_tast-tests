// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Config,
		Desc: "Verifies that remote tests can load fw-testing-configs properly",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Attr:         []string{"group:mainline", "group:firmware", "firmware_smoke"},
		SoftwareDeps: []string{"crossystem", "chromeos_firmware"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func Config(ctx context.Context, s *testing.State) {
	h := firmware.NewHelper(s.DUT(), s.RPCHint(), s.DataPath(firmware.ConfigFile), "", "", "", "", "")
	defer func() {
		if err := h.Close(ctx); err != nil {
			s.Fatal("Closing helper: ", err)
		}
	}()

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create firmware config: ", err)
	}

	// Verify that the loaded config's "platform" attribute matches the board/variant fetched by the helper.
	expectedPlatform := firmware.CfgPlatformFromLSBBoard(h.Board)
	if h.Config.Platform != expectedPlatform {
		s.Errorf("Unexpected Platform value; got %s, want %s", h.Config.Platform, expectedPlatform)
	}
}
