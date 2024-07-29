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
		Func:         LocalRun,
		Desc:         "Runs lldb to verify it built and can open correctly",
		Contacts:     []string{"c-compiler-chrome@google.com", "ajordanr@google.com"},
		BugComponent: "b:1038090",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"lldb"},
		Timeout:      time.Minute,
	})
}

func LocalRun(ctx context.Context, s *testing.State) {
	localCmd := testexec.CommandContext(ctx, "/usr/local/bin/lldb", "<<EOF\nexit\nEOF")
	out, err := localCmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("executing lldb failed with output %q: %v", out, err)
	}
}
