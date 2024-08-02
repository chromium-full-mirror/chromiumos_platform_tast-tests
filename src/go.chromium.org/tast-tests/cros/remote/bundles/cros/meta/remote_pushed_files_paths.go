// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemotePushedFilesPaths,
		Desc:         "Example to get information on files pushed by Tast",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
	})
}

func RemotePushedFilesPaths(ctx context.Context, s *testing.State) {
	pushedFiles := s.PushedFilesToDUT("")
	s.Logf("Following files have been pushed to the DUT by Tast %+v", pushedFiles)
}
