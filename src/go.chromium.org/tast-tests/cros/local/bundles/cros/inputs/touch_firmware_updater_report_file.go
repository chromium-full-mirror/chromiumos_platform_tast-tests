// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"os"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TouchFirmwareUpdaterReportFile,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validates touch firmware information is always available after boot and not empty",
		Contacts: []string{
			"chromeos-tango@google.com",
			"maek@google.com", // Test author
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:167253", // ChromeOS > Platform > baseOS > Input > Touchpad
	})
}

const (
	// FirmwareReportFilePath is the path to Firmware Report on Devices.
	firmwareReportFilePath = "/run/touch-updater/firmware-versions"
)

func TouchFirmwareUpdaterReportFile(ctx context.Context, s *testing.State) {
	fileStats, err := os.Stat(firmwareReportFilePath)
	if err != nil {
		s.Fatal("Failed to read touch firmware report file: ", err)
	}

	// Check if firmware file is empty.
	if fileStats.Size() == 0 {
		s.Fatal("Failed because touch firmware report file is empty")
	}

	// TODO(b/310056795): Validate File Entries.

}
