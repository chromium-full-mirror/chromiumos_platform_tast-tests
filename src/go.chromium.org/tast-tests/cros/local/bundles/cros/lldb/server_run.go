// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lldb

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ServerRun,
		Desc:         "Runs lldb-server to verify it built and can open correctly",
		Contacts:     []string{"c-compiler-chrome@google.com", "ajordanr@google.com"},
		BugComponent: "b:1038090",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      time.Minute,
	})
}

func ServerRun(ctx context.Context, s *testing.State) {
	serverCmd := testexec.CommandContext(ctx, "/usr/local/bin/lldb-server")
	out, err := serverCmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("executing lldb-server failed with output %q: %v", out, err)
	}
	serverCmd.Kill()
}
