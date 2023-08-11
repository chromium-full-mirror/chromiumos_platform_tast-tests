// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package factory

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/factory/fixture"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Toolkit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test if toolkit is running",
		Contacts:     []string{"chromeos-factory-eng@google.com", "lschyi@google.com"},
		// ChromeOS > Platform > Enablement > Factory
		BugComponent: "b:167224",
		Attr:         []string{"group:mainline"},
		Timeout:      time.Minute,
		Fixture:      fixture.EnsureToolkit,
		// Skip "nyan_kitty" due to slow reboot speed.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("kitty")),
		SoftwareDeps: append([]string{"factory_flow"}, fixture.EnsureToolkitSoftwareDeps...),
	})
}

func Toolkit(ctx context.Context, s *testing.State) {
	conn := s.DUT().Conn()
	probeTestListCmd := conn.CommandContext(ctx, "factory", "test-list")
	if err := probeTestListCmd.Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to run toolkit: ", err)
	}
}
