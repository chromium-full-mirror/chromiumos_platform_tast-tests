// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LibDRM,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A quick smoke check using the binaries shipping with libdrm",
		// TODO(syedfaaiz): Add to CQ once it is green and stable.
		Attr:         []string{"group:graphics", "group:mainline", "graphics_nightly", "informational"},
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Contacts: []string{
			"chromeos-gfx@google.com",
			"syedfaaiz@google.com",
		},
		Fixture: "graphicsNoChrome",
		Params: []testing.Param{{
			Name:    "kmstest",
			Val:     []string{"kmstest"},
			Timeout: 2 * time.Minute,
		}, {
			Name:    "modetest",
			Val:     []string{"modetest"},
			Timeout: 2 * time.Minute,
		}, {
			Name:    "proptest",
			Val:     []string{"proptest"},
			Timeout: 2 * time.Minute,
		}},
	})
}

func LibDRM(ctx context.Context, s *testing.State) {
	test := s.Param().([]string)[0]
	err := testexec.CommandContext(ctx, "which", test).Run(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("Command %v doesn't exist: %v", test, err)
	}
	_, stderr, err := testexec.CommandContext(ctx, test).SeparatedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("Failed to run %v: %v", string(stderr), err)
	}
}
