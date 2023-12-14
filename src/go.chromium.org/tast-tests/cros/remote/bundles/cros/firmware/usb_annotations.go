// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
		// TODO: when stable, move to firmware_ec
		Attr: []string{"group:firmware", "firmware_unstable"},
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

// TODO: confirm currently un-annotated platforms. below is a list of known
//
//	annotated ones.
//
//	var AnnotatedPlatforms = []string{
//		"brya",
//		"dedede",
//		"guybrush",
//		"hatch",
//		"nissa",
//		"octopus",
//		"puff",
//		"rex",
//		"skolas",
//		"skyrim",
//		"volteer",
//		"zork",
//	}
var unannotatedPlatforms = []string{
	"bob",
	"elm",
	"fievel",
	"hana",
	"herobrine",
	"kevin",
	"kevin64",
	"oak",
	"tiger",
}
