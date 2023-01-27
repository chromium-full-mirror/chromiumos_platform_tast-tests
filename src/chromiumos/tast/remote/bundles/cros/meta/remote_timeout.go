// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

const timeout = 2 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:     RemoteTimeout,
		Desc:     "Always times out",
		Contacts: []string{"tast-owners@google.com"},
		Timeout:  timeout,
		// ChromeOS > Test > Harness > Tast > Framework
		BugComponent: "b:1034754",
	})
}

func RemoteTimeout(ctx context.Context, s *testing.State) {
	testing.Poll(ctx, func(context.Context) error {
		s.Log("Polling until timeout")
		return errors.New("Keep the Poll alive")
	}, &testing.PollOptions{
		Timeout:  timeout + time.Minute,
		Interval: 30 * time.Second,
	})
}
