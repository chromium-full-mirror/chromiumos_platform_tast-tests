// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package starfish

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Basic,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies basic Starfish functionality",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular"},
		Fixture:      "starfish",
		Timeout:      1 * time.Minute,
		SoftwareDeps: []string{"chrome"},
	})
}

// Basic test to verify if the Starfish module is being initialized successfully.
func Basic(ctx context.Context, s *testing.State) {
	testing.ContextLog(ctx, "Done")
}
