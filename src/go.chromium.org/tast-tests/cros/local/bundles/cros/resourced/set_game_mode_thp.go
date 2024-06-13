// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package resourced

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast-tests/cros/local/resourced/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetGameModeThp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks swappiness and thp tuning when resourced game mode changes",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      2 * time.Minute,
	})
}

func SetGameModeThp(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	if err := utils.CheckSetGameMode(ctx, rm, true, true); err != nil {
		s.Fatal("Checking swappiness/THP tuning with SetGameMode failed: ", err)
	}
}
