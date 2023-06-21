// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OnErrorExample,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "An example of setting error handling functions",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func OnErrorExample(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	s.AttachErrorHandlers(
		func(errMsg string) {
			testing.ContextLog(ctx, "Error handler is called")
			faillog.DumpUITreeOnErrorToFile(ctx, s.OutDir(), s.HasError, tconn, "uitree_error.txt")
		},
		func(errMsg string) {
			testing.ContextLog(ctx, "Fatal handler is called")
			faillog.DumpUITreeOnErrorToFile(ctx, s.OutDir(), s.HasError, tconn, "uitree_fatal.txt")
		})
	s.Error("Intentinonally failed")

	// GoBigSleepLint: sleep for a few second  so that ui_tree files have different timestamp.
	if err := testing.Sleep(ctx, time.Second*15); err != nil {
		s.Error("Failed to sleep")
	}
	s.Fatal("Intentionally failed fatally")
}
