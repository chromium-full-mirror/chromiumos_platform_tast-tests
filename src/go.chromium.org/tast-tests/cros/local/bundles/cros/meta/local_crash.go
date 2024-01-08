// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalCrash,
		Desc:         "Always crashes",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:hw_agnostic"},
	})
}

func LocalCrash(ctx context.Context, s *testing.State) {
	// Panic on a goroutine so that it is not recovered by the framework.
	go panic("crashing")
	var ch chan struct{}
	<-ch
}
