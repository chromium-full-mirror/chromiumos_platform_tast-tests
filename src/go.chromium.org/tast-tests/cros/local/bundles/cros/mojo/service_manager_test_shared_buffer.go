// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mojo tests the system daemon mojo_service_manager to verify the mojo
// functionality.
package mojo

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/mojo/constants"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ServiceManagerTestSharedBuffer,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check we can create shared buffer from mojo service manager",
		Contacts: []string{
			"chromeos-mojo-service-manager@google.com",
			"chungsheng@google.com",
		},
		// ChromeOS > Platform > Services > Mojo Service Manager
		BugComponent: "b:1188058",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      1 * time.Minute,
	})
}

// ServiceManagerTestSharedBuffer tests the shared buffer can be created.
func ServiceManagerTestSharedBuffer(ctx context.Context, s *testing.State) {
	sharedBufProc := testexec.CommandContext(ctx, constants.TestToolBinary, "--action=test-shared-buffer")
	if err := sharedBufProc.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to create shared buffer: ", err)
	}
}
