// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrosID,
		Desc:         "Tests that unibuild devices are able to probe identity via crosid",
		Contacts:     []string{"chromeos-config@google.com", "jrosenth@chromium.org", "chromeos-faft@google.com"},
		BugComponent: "b:970794", // ChromeOS > Platform > Enablement > Firmware > unibuild
		SoftwareDeps: []string{"unibuild"},
		Attr:         []string{"group:mainline", "group:firmware", "firmware_ec"},
		Requirements: []string{"sys-fw-0022-v02"},
	})
}

func CrosID(ctx context.Context, s *testing.State) {
	out, err := testexec.CommandContext(ctx, "crosid", "-v").CombinedOutput()
	if err != nil {
		s.Fatalf("crosid failed: %+v %s", err, string(out))
	}
}
