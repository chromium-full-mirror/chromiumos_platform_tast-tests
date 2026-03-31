// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This test is designed to test that ChromeOS platforms have implemented a
// minimal set of annotations for their USB ports. Test failure would be
// indicative of missing annotations, most notably UserVisible field in _PLD
// table and PortIsConnectable field in _UPC table.

// https://learn.microsoft.com/en-us/windows-hardware/drivers/install/using-acpi-to-configure-usb-ports-on-a-computer
// contains good reference material.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	usbSysfsRootGlob = "/sys/bus/usb/devices/*"
	portRemovable    = "removable"
	portFixed        = "fixed"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: USBAnnotations,
		Desc: "Confirms that USB port annotations have been provided from firmware",
		Contacts: []string{
			"clumptini@google.com",
			"drmasquatch@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		// Based on known un-annotated platforms that might not be updated.
		HardwareDeps: hwdep.D(hwdep.SkipOnPlatform(unannotatedPlatforms...), hwdep.X86()),
	})
}

func USBAnnotations(ctx context.Context, s *testing.State) {
	files, err := filepath.Glob(usbSysfsRootGlob)
	if err != nil {
		s.Fatal("Could not find any files in USB sysfs: ", err)
	}

	foundAnyAnnotatedPort := false

	for _, file := range files {
		s.Log("looking at ", file)

		// We don't care about errors here, as long as we are able to find at
		// least one correctly annotated port that means we've gotten
		// information from fw.
		contents, _ := os.ReadFile(file + "/removable")
		text := strings.TrimRight(string(contents), "\r\n")

		s.Log(file + "/removable = " + text)

		if text == portRemovable || text == portFixed {
			foundAnyAnnotatedPort = true
			break
		}
	}

	if !foundAnyAnnotatedPort {
		s.Fatal("Did not find any annotated ports")
	}
}

var unannotatedPlatforms = []string{
	"coral",
	"elm",
	"grunt",
	"hana",
	"nami",
	"nautilus",
	"oak",
	"reef",
	"sand",
	"snappy",
}
