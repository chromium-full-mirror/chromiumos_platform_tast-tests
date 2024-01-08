// Copyright 2020 The ChromiumOS Authors
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
		Func:         RemoteFreeze,
		Desc:         "Always freezes",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Timeout:      time.Second,
		Attr:         []string{"group:hw_agnostic"},
	})
}

func RemoteFreeze(ctx context.Context, s *testing.State) {
	var ch chan struct{}
	<-ch
}
