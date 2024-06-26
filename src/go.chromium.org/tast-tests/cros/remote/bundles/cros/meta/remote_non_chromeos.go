// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteNonChromeOS,
		Desc:         "Example of running a test on a non-ChromeOS device",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		VarDeps:      []string{"meta.non_chrome_dut_host"},
		Attr:         []string{"group:hw_agnostic"},
		// This test is called by remote tests in the meta package.
	})
}

var nonChromeOSDUTHost = testing.RegisterVarString(
	"meta.non_chrome_dut_host",
	"",
	"The hostname of the non-ChromeOS DUT",
)

// RemoteNonChromeOS is an example on how to run test against a
// non-ChromeOS device.
// Example: tast run -var=meta.non_chrome_dut_host=localhost:2226 - meta.RemoteNonChromeOS
func RemoteNonChromeOS(ctx context.Context, s *testing.State) {
	hostname := nonChromeOSDUTHost.Value()
	if hostname == "" {
		s.Fatal("It is required to specify meta.non_chrome_dut_host to run this test")
	}
	// This example assume that key file are in ~/.ssh.
	kf := filepath.Join(os.Getenv("HOME"), ".ssh", "testing_rsa")
	if _, err := os.Stat(kf); err != nil {
		kf = ""
	}

	kd := filepath.Join(os.Getenv("HOME"), ".ssh")
	if _, err := os.Stat(kd); err != nil {
		kd = ""
	}

	s.Logf("Key File: %q, Key Dir: %q", kf, kd)

	var sopt ssh.Options
	ssh.ParseTarget(hostname, &sopt)
	sopt.KeyDir = kd
	sopt.KeyFile = kf
	sopt.ConnectTimeout = 10 * time.Second
	conn, err := ssh.New(ctx, &sopt)
	if err != nil {
		s.Fatalf("Failed to connect to %s: %v", hostname, err)
	}
	defer conn.Close(ctx)
	out, err := conn.CommandContext(ctx, "date").Output()
	if err != nil {
		s.Fatalf("Failed to run date on host %s: %v", hostname, err)
	}
	s.Logf("The current time at %s is %s", hostname, out)
}
