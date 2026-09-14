// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"os"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     KmodRunModprobe,
		Desc:     "Verifies that kmod does not parse /run/modprobe.d",
		Contacts: []string{"chromeos-hardening@google.com"},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		Attr:         []string{"group:mainline", "informational"},
	})
}

func KmodRunModprobe(ctx context.Context, s *testing.State) {
	const (
		modprobeDir = "/run/modprobe.d"
		installConf = "/run/modprobe.d/install_wireguard.conf"
		installLine = "install wireguard /bin/touch /tmp/touched_by_modprobe\n"
		touchedFile = "/tmp/touched_by_modprobe"
	)

	// Attempt to unload the wireguard module first. If the module is already loaded
	// from a prior test, kmod won't be invoked, which could lead to a false pass.
	// It's safe to ignore the error if it's not currently loaded.
	testexec.CommandContext(ctx, "modprobe", "-r", "wireguard").Run()

	// Clean up the environment before the test.
	os.RemoveAll(modprobeDir)
	os.Remove(touchedFile)

	// Ensure cleanup is executed after the test finishes.
	defer func() {
		testexec.CommandContext(ctx, "modprobe", "-r", "wireguard").Run()
		os.RemoveAll(modprobeDir)
		os.Remove(touchedFile)
	}()

	// 1. mkdir -p /run/modprobe.d (as root)
	if err := os.MkdirAll(modprobeDir, 0755); err != nil {
		s.Fatalf("Failed to create %s: %v", modprobeDir, err)
	}

	// 2. Write install config and chmod 644
	if err := os.WriteFile(installConf, []byte(installLine), 0644); err != nil {
		s.Fatalf("Failed to write to %s: %v", installConf, err)
	}

	// 3. Execute the module loading command as the chronos user.
	s.Log("Triggering unshare to load the wireguard module as chronos")
	cmd := testexec.CommandContext(ctx, "sudo", "-u", "chronos", "unshare", "-r", "-n", "ip", "link", "add", "dev", "wg0", "type", "wireguard")
	if err := cmd.Run(); err != nil {
		// Expecting this might return an error if wireguard can't be created cleanly,
		// or if kmod actively blocks the process. We just care if kmod executed the touch.
		s.Log("ip link command returned an error: ", err)
	}

	// 4. Verify /tmp/touched_by_modprobe does not exist.
	if _, err := os.Stat(touchedFile); err == nil {
		s.Fatalf("%q was created by kmod, meaning /run/modprobe.d/ was parsed", touchedFile)
	} else if !os.IsNotExist(err) {
		s.Fatalf("Failed to check for %q: %v", touchedFile, err)
	}
}
