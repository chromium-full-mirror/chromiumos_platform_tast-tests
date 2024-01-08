// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalFreezeForever,
		Desc:         "Always freezes forever",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Timeout:      100 * time.Hour,
		Attr:         []string{"group:hw_agnostic"},
	})
}

func LocalFreezeForever(ctx context.Context, s *testing.State) {
	// Log for meta test to detect start of the test.
	s.Log("LocalFreezeForever started")
	var ch chan struct{}
	<-ch
}
