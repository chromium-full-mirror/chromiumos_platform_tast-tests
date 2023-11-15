// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Flashrom,
		Desc: "Checks that flashrom can find a SPI ROM",
		Contacts: []string{
			"cros-flashrom-team@google.com", // CrOS Flashrom team
			"chromeos-firmware@google.com",  // CrOS Firmware Developers
			"quasisec@chromium.org",         // CrOS Flashrom Maintainer
		},
		BugComponent: "b:750299", // ChromeOS > Platform > Enablement > Firmware > Flashrom
		Attr:         []string{"group:mainline", "group:labqual"},
		SoftwareDeps: []string{"flashrom", "chromeos_firmware"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// Flashrom runs the flashrom utility and confirms that flashrom was able to
// communicate with a SPI ROM.
func Flashrom(ctx context.Context, s *testing.State) {
	// This test intentionally avoids SPI ROM read and write operations, so as not
	// to stress devices-under-test.
	var flashromConfig flashrom.Config
	_, ctx, shutdown, out, err := flashromConfig.
		FlashromInit(flashrom.VerbosityDebug).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		Probe(ctx)

	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()

	if err != nil {
		path := filepath.Join(s.OutDir(), "flashrom.txt")
		if err := ioutil.WriteFile(path, out, 0644); err != nil {
			s.Error("Failed to save flashrom output: ", err)
		}
		s.Fatalf("Failed to confirm flashrom could find a flash chip:  "+
			"%v (saved output to %s).",
			err, filepath.Base(path))
	}
}
