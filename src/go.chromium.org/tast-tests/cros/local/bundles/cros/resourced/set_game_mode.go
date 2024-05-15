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
		Func:         SetGameMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced setting game mode",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline"},
		Timeout:      2 * time.Minute,
	})
}

func SetGameMode(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	if err := utils.CheckSetGameMode(ctx, rm, false, false); err != nil {
		s.Fatal("Checking SetGameMode failed: ", err)
	}
}
