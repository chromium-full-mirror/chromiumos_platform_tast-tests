// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChameleonSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that a working Chameleon device is connected to at least one valid port",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		Attr: []string{
			"group:graphics",
			"graphics_chameleon_igt",
			"graphics_nightly",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"graphics.chameleon_ip"},
		Fixture:      "gpuWatchHangs",
		Timeout:      chrome.LoginTimeout + time.Minute,
	})
}

func ChameleonSmoke(ctx context.Context, s *testing.State) {
	validPorts := 0
	// Log into Chrome to ensure there is something on the screen.
	// Strictly speaking Chrome is not needed for this test.
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)

	conn, err := cr.NewConn(ctx, "chrome://gpu")
	if err != nil {
		s.Fatal("Failed to load chrome://gpu: ", err)
	}
	defer conn.Close()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("outDir is not available")
	}

	// TODO(ihf|markyacoub): consider creating a fixture for each port so
	// that all Chameleon tests can focus on testing the active ports only
	// and ignore inactive ones.
	cham, err := graphics.ChameleonGetConnection(ctx)
	if err != nil {
		s.Fatal("Failed to get the Chameleond instance: ", err)
	}
	supportedPorts, err := cham.GetSupportedPorts(ctx)
	if err != nil {
		s.Fatal("Failed to get supported ports: ", err)
	}

	for _, port := range supportedPorts {
		shouldUsePort, err := graphics.ChameleonShouldUsePort(ctx, cham, port)
		if err != nil {
			s.Fatalf("Failed to determine if plug can be used for port %d: %s", port, err)
		}
		if !shouldUsePort {
			s.Logf("Chameleon is not plugged into port %d", port)
			continue
		}
		if err = graphics.ChameleonPlug(ctx, cham, port); err != nil {
			s.Fatalf("Failed to get stable video input from a physically plugged port %d: %s", port, err)
		}

		// TODO(b:261600622): Replace ChameleonGetScreenshot with screen-util-tools command.
		curChamPath := filepath.Join(outDir, "cham_"+strconv.Itoa(int(port))+".png")
		if err = graphics.ChameleonGetScreenshot(ctx, cham, port, curChamPath); err != nil {
			s.Errorf("Cannot get Chameleon screenshot on port %d: %s", port, err)
		}

		if err = cham.Unplug(ctx, port); err != nil {
			s.Fatalf("Failed to unplug a physically plugged port %d: %s ", port, err)
		}
		validPorts++
	}

	s.Logf("Found %d valid Chameleon ports", validPorts)
	if validPorts == 0 {
		s.Error("No working Chameleon device found")
	}
}
