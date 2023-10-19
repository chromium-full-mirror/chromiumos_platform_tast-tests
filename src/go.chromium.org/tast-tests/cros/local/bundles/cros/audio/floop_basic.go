// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FloopBasic,
		Desc: "Chcek cras_test_client --request_floop_mask works",
		Contacts: []string{
			"chromeos-audio-bugs@google.com",
			"aaronyu@google.com",
			"htcheong@google.com",
		},
		Fixture:      fixture.FakeCrasClient,
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
		},
	})
}

func FloopBasic(ctx context.Context, s *testing.State) {
	const (
		blockSize    = 480
		FloopTimeout = 10 * time.Second
	)

	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Cannot restart CRAS: ", err)
	}

	dev, err := crastestclient.RequestFloopMask(ctx, 1)
	if err != nil {
		s.Fatal("RequestFloopMask failed for mask=1: ", err)
	}

	testing.ContextLogf(ctx, "Record on loopback device %d for 3 seconds", dev)
	cmd := crastestclient.PinCaptureCommand(ctx, dev, 3, 480)
	if err := cmd.Run(exec.DumpLogOnError); err != nil {
		s.Fatal("Record from loopback device failed: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err = cras.GetNodeByType(ctx, "FLEXIBLE_LOOPBACK"); err != nil {
			return nil
		}
		return errors.New("Flexible loopback device can still found")
	}, &testing.PollOptions{Interval: time.Second, Timeout: 2 * FloopTimeout}); err != nil {
		s.Fatal("Check for flexible loopback device self-destroy failed: ", err)
	}

	dev1, err := crastestclient.RequestFloopMask(ctx, 1)
	if err != nil {
		s.Fatal("RequestFloopMask failed for mask=1: ", err)
	}

	tempDev, err := crastestclient.RequestFloopMask(ctx, 1)
	if err != nil {
		s.Fatal("RequestFloopMask failed for mask=1: ", err)
	}
	if tempDev != dev1 {
		s.Errorf("consecutive RequestFloopMask with same masks returned different devices: first -> %d; second -> %d",
			dev1, tempDev,
		)
	}

	dev2, err := crastestclient.RequestFloopMask(ctx, 2)
	if err != nil {
		s.Fatal("RequestFloopMask failed for mask=2: ", err)
	}

	if dev2 == dev1 {
		s.Errorf("consecutive RequestFloopMask with different masks returned the same device: mask=1 -> %d; mask=2 -> %d",
			dev1, dev2,
		)
	}
}
