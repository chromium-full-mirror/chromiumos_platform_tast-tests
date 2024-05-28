// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bisector contains custom tast test for Bisector provided by the users
package bisector

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Bisect,
		Desc:         "Stub test ready to be modified for bisection",
		Contacts:     []string{"chromeos-bisector-eng@google.com", "njrafi@chromium.org"},
		BugComponent: "b:497108", // ChromeOS > Platform > Technologies > Health Monitoring > Bisector

		// This test is disabled and will not be run by any automation.
		// Users will modify the test in their CL which will be used by Bisector to run bisections.
		// However those CLs will be never merged to the source tree.
		Attr: []string{},
	})
}

func Bisect(ctx context.Context, s *testing.State) {
	// No errors means the test passed.
}
