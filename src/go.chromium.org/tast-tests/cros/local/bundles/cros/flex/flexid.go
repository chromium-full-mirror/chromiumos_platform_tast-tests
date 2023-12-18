// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"unicode"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FlexID,
		Desc: "Tests that the go/chromeos-flex-id is properly-formed",
		Contacts: []string{
			"chromeos-flex-eng@google.com",
			"josephsussman@google.com", // Test author
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		SoftwareDeps: []string{"flex_id"},
		Attr:         []string{"group:mainline"},
	})
}

// isAllASCII returns true if output only contains ASCII characters.
func isAllASCII(output string) bool {
	for i := 0; i < len(output); i++ {
		if output[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func FlexID(ctx context.Context, s *testing.State) {
	out, err := testexec.CommandContext(ctx, "flex_id_tool").CombinedOutput()
	status, ok := testexec.ExitCode(err)

	outString := string(out)
	if !ok {
		s.Fatalf("flex_id_tool exited with status %d: %s", status, outString)
	}

	if !isAllASCII(outString) {
		s.Errorf("%s contains non-ASCII characters", outString)
	}
}
