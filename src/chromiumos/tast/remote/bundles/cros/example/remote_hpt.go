// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"
	"time"

	ps "go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteHPT,
		Desc:         "Demonstrates how to use HPT Service in a remote test",
		Contacts:     []string{"hpt-project-team@google.com", "acwans@google.com"},
		BugComponent: "b:1094001", // ChromeOS > EngProd > Platform > Base OS
		Attr:         []string{"group:mainline", "informational"},
		ServiceDeps:  []string{"tast.cros.wifi.HPTService"},
	})
}

func RemoteHPT(ctx context.Context, s *testing.State) {
	var outDir string = "/var/tmp/"
	var hz int32 = 4000

	// Establish RPC connection to the DUT for HPT service
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	defer cl.Close(ctx)
	psc := ps.NewHPTServiceClient(cl.Conn)

	//Start perf record
	if _, err = psc.StartHPT(ctx, &ps.HPTRequest{
		OutDir:       outDir,
		SampleRateHz: hz,
	}); err != nil {
		s.Fatal("Failed to start trace : ", err)
	}

	defer func() {
		// Stop perf record
		if _, err = psc.StopHPT(ctx, &ps.HPTRequest{}); err != nil {
			s.Log("Failed to stop trace : ", err)
		} else {
			s.Log("Trace saved")
		}
	}()

	// Wait for 5 seconds to gather data.
	// In a real test (not an example), test content would go here instead of
	// sleeping.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failure in sleeping: ", err)
	}
}
