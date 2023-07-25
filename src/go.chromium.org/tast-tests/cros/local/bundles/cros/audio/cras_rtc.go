// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasRTC,
		Desc:         "Check checks RTC status reporting",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		Timeout:      1 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Val: crasRTCParam{
					expectCallOnRTCStart: &dbusutil.CalledMethod{
						MethodName: "SetRTCAudioActive",
						Arguments:  []interface{}{uint8(1)},
					},
					expectCallOnRTCStop: &dbusutil.CalledMethod{
						MethodName: "SetRTCAudioActive",
						Arguments:  []interface{}{uint8(0)},
					},
				},
			},
		},
	})
}

type crasRTCParam struct {
	expectCallOnRTCStart *dbusutil.CalledMethod
	expectCallOnRTCStop  *dbusutil.CalledMethod
}

func CrasRTC(ctx context.Context, s *testing.State) {
	param := s.Param().(crasRTCParam)

	if _, err := audio.RestartCras(ctx); err != nil {
		s.Fatal("Cannot restart CRAS")
	}

	makeEventWatcher := func() (*dbusutil.EventWatcher, context.CancelFunc) {
		evCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		ew, err := dbusutil.NewEventWatcher(evCtx, []dbusutil.MatchSpec{{
			Path:      "/org/chromium/ResourceManager",
			Interface: "org.chromium.ResourceManager",
			Member:    "SetRTCAudioActive",
		}})
		if err != nil {
			s.Fatal("Cannot set up D-Bus event watcher: ", err)
		}
		return ew, cancel
	}

	// Start watching for SetRTCAudioActive.
	ew, cancel := makeEventWatcher()
	defer cancel()
	defer ew.Close()

	// Start audio playback and capture.
	clientCtx, clientCancel := context.WithCancel(ctx)
	defer clientCancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func(ctx context.Context) {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, "cras_test_client", "-C", "/dev/null", "--rate=48000", "--block_size=480")
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			s.Error("Capture failed: ", err)
		}

	}(clientCtx)
	go func(ctx context.Context) {
		defer wg.Done()
		cmd := exec.CommandContext(ctx, "cras_test_client", "-P", "/dev/zero", "--rate=48000", "--block_size=480")
		if err := cmd.Run(); err != nil && ctx.Err() == nil {
			s.Error("Playback failed: ", err)
		}
	}(clientCtx)

	// Check SetRTCAudioActive.
	if diff := cmp.Diff(param.expectCallOnRTCStart, <-ew.Events()); diff != "" {
		s.Fatalf("Unexpected D-Bus call on start: -want +got:%s%s", "\n", diff)
	}

	// Watch for SetRTCAudioActive on stop.
	ew, cancel = makeEventWatcher()
	defer cancel()
	defer ew.Close()

	// Stop audio playback and capture.
	clientCancel()
	wg.Wait()

	// Check SetRTCAudioActive again after stopping streams.
	if diff := cmp.Diff(param.expectCallOnRTCStop, <-ew.Events()); diff != "" {
		s.Fatalf("Unexpected D-Bus call on stop: -want +got:%s%s", "\n", diff)
	}
}
