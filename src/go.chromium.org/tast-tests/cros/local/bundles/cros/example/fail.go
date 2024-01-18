// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Fail,
		Desc:         "Always fails",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
	})
}

func Fail(ctx context.Context, s *testing.State) {
	s.Log("Here's an informative message")
	s.Error("Here's an error")
	s.Error("And here's a second")
	s.Fatal("Finally, a fatal error")
}
