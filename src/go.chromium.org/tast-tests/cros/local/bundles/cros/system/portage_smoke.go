// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package system

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PortageSmoke,
		Desc: "Makes sure the emerge command is usable; needed for cros deploy",
		Contacts: []string{
			"chromeos-build-team@google.com",
			"aaronyu@google.com", // test author
		},
		BugComponent: "b:1037860", // ChromeOS Public Tracker > Services > Infra > Build
		Attr:         []string{"group:mainline", "informational"},
	})
}

func PortageSmoke(ctx context.Context, s *testing.State) {
	if err := testexec.CommandContext(ctx, "emerge", "--version").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("emerge --version failed: ", err)
	}
}
