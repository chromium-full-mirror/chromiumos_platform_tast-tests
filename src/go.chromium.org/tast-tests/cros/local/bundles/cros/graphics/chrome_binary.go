// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChromeBinary,
		Desc: "This test runs chrome binary tests and reports errors ",
		Contacts: []string{
			"chromeos-gfx@google.com",
			"jshargo@google.com",
			"mcasas@google.com",
			"syedfaaiz@google.com",
		},
		BugComponent: "b:1021073",
		Attr:         []string{"graphics_perbuild", "group:graphics"},
		Timeout:      2 * time.Minute,
		Fixture:      "graphicsNoChrome",
		Params: []testing.Param{{
			Name: "ozone_gl_unittests",
			Val:  "ozone_gl_unittests",
		}},
	})
}

func ChromeBinary(ctx context.Context, s *testing.State) {
	binary := s.Param().(string)
	// Run the test
	err := testexec.CommandContext(ctx, chrome.BinTestDir+binary).Run(testexec.DumpLogOnError)
	if err == nil {
		s.Fatalf("Failed to run %s: %s ", binary, err)
	}
}
