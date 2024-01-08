// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalPoll,
		Desc:         "Intentionally fails due to timeout",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:hw_agnostic"},
	})
}

func LocalPoll(ctx context.Context, s *testing.State) {
	// CAUTION: when using the Poll utility, errors should be handled correctly.
	// Here, the test is expecting a timeour error because the passed in function
	// always returns an error.
	timeout := time.Second + 2*time.Millisecond + 3*time.Nanosecond
	opts := &testing.PollOptions{
		Timeout:  timeout,
		Interval: time.Millisecond,
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return errors.New("error to force to timeout")
	}, opts); err != nil {
		s.Logf("Get expected error from poll: %s", err)
	} else {
		s.Error("Error with poll: ", err)
	}
}
