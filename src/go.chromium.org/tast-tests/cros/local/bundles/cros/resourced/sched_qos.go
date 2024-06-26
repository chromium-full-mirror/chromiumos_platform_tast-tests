// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package resourced

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast-tests/cros/local/sched"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SchedQos,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced setting thread/process QoS state",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"pidfd_open"},
		Timeout:      2 * time.Minute,
	})
}

func SchedQos(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	p, err := sched.CreateSampleProcessThreadPair(ctx, nil)
	if err != nil {
		s.Fatal("Failed to create process: ", err)
	}
	defer p.KillAndWait()

	if err := rm.SetProcessState(ctx, p.Pid, resourced.QoSProcessNormal); err != nil {
		s.Fatal("Failed to set process state: ", err)
	}
	for _, state := range []uint8{resourced.QoSThreadUrgentBursty, resourced.QoSThreadUrgent, resourced.QoSThreadBalanced, resourced.QoSThreadEco, resourced.QoSThreadUtility, resourced.QoSThreadBackground} {
		if err := rm.SetThreadState(ctx, p.Pid, p.Tid, state); err != nil {
			s.Fatal("Failed to set thread state: ", err)
		}
	}
	if err := rm.SetProcessState(ctx, p.Pid, resourced.QoSProcessBackground); err != nil {
		s.Fatal("Failed to set process state: ", err)
	}
}
