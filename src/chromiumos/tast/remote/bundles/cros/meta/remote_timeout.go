// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"time"

	"chromiumos/tast/testing"
)

const timeout = 2 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:     RemoteTimeout,
		Desc:     "Always times out",
		Contacts: []string{"tast-core@google.com"},
		Timeout:  timeout,
		// ChromeOS > Test > Harness > Tast > Examples
		BugComponent: "b:1034522",
	})
}

func RemoteTimeout(ctx context.Context, s *testing.State) {
	sleepTime := timeout + time.Minute
	s.Logf("Sleeping for %v to force test timeout(%v) to be reached ", sleepTime, timeout)
	sleepContext, cancel := context.WithTimeout(context.Background(), sleepTime)
	defer cancel()
	testing.Sleep(sleepContext, sleepTime)
	s.Error("Error: Sleep complete without test timeout expiring")
}
