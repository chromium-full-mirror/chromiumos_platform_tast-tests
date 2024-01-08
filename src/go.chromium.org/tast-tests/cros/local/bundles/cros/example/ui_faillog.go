// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UIFaillog,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check if faillog for the UI tree works",
		Contacts:     []string{"tast-core@google.com", "hidehiko@chromium.org"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

func UIFaillog(ctx context.Context, s *testing.State) {
	// To make sure brand new Chrome instance, do not use fixture.
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to log in ash-chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get tconn: ", err)
	}

	s.Log("Dump the UI tree to ui_dump.txt")
	filePath := filepath.Join(s.OutDir(), "ui_dump.txt")
	if err := uiauto.LogRootDebugInfo(ctx, tconn, filePath); err != nil {
		s.Fatal("Failed to dump: ", err)
	}

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		s.Fatal("Failed to read ui_dump file: ", err)
	}

	if len(b) == 0 {
		s.Fatal("Dump file is unexpectedly empty")
	}
}
