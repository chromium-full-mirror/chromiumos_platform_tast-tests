// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerOff,
		Desc:         "Shut down a DUT to simulate a DUT losing connectivity",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
	})
}

// PowerOff turns off power to simulate a DUT losing connectivity.
func PowerOff(ctx context.Context, s *testing.State) {
	cmd := testexec.CommandContext(ctx, "/sbin/shutdown", "-h", "now")
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}
	<-ctx.Done()
}
