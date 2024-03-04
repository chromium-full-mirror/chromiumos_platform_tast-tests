// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/compliance"
	"go.chromium.org/tast-tests/cros/remote/compliance/ex350"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ComplianceSmoke,
		Desc:     "Meta test that verifies that we're able to connect to a compliance tester host, run a simple command, and copy a file back",
		Contacts: []string{"chromeos-usb@google.com", "jstanko@chromium.org"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec"},
		Fixture:      "complianceHostFixture",
		Timeout:      5 * time.Minute,
	})
}

func ComplianceSmoke(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*compliance.TestFixture)

	controller := ex350.NewController(fixt.Host)
	temp, remove, err := controller.Host.CreateTempDirectory(ctx)
	if err != nil {
		s.Fatal("Failed to create temp file: ", err)
	}
	defer remove(ctx)
	s.Log("Created temp file: ", temp)

	// Note: be careful using string formatting when constructing windows file paths,
	// they can end up with additional escapes which aren't interpreted correctly on
	// the Windows host machines, so it's easier to just use normal concat.
	message := "Hello " + time.Now().Format(time.UnixDate)
	if err := controller.Host.Run(ctx, "echo", message, ">", temp+"\\date.txt"); err != nil {
		s.Fatal("Failed to write to temp directory: ", err)
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get out dir")
	}

	if err := controller.Host.GetFile(ctx, temp+"\\*", outDir); err != nil {
		s.Fatal("Failed to fetch file from compliance host: ", err)
	}
}
