// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"encoding/hex"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FileSystemXattrs,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies filesystem extended attributes for ARC container",
		Contacts:     []string{"arc-core@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		// TODO(yusukes,ricardoq): ARCVM does not need the test. Remove this once we retire ARC container.
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Attr:         []string{"group:mainline"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "container_r",
			ExtraSoftwareDeps: []string{"android_container_r"},
		}},
	})
}

func FileSystemXattrs(ctx context.Context, s *testing.State) {
	const (
		path = "/opt/google/containers/android/rootfs/root/system/bin/run-as"
		key  = "security.capability"
		// security.capability with CAP_SETUID and CAP_SETGID encoded in hex.
		expect = "01000002c0000000000000000000000000000000"
	)

	out, err := testexec.CommandContext(ctx, "getfattr", "--only-values", "--name", key, path).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to get %s xattr for %s: %v", key, path, err)
	}
	if val := hex.EncodeToString(out); val != expect {
		s.Fatalf("Unexpected %s xattr for %s: got %s; want %s", key, path, val, expect)
	}
}
