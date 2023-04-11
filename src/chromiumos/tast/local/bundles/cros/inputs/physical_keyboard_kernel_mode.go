// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardKernelMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Internal keyboard should not be in AT Raw mode",
		Contacts:     []string{"chromeos-tango@google.com", "kenalba@google.com", "dtor@google.com", "dnojiri@google.com"},
		BugComponent: "b:167212", // ChromeOS > Platform > Services > Input
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		Timeout:      3 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "chromeLoggedIn",
		HardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.FormFactor(hwdep.Convertible, hwdep.Clamshell, hwdep.Detachable)),
	})
}

func PhysicalKeyboardKernelMode(ctx context.Context, s *testing.State) {
	keyboardAvailable, keyboardName, err := input.FindPhysicalKeyboardName(ctx)
	if err != nil {
		s.Fatal("Unable to run test without physical keyboard information: ", err)
	}

	if !keyboardAvailable {
		s.Fatal("Unable to run test without keyboard attached and available")
	}

	if keyboardName == "AT Raw Set 2 keyboard" {
		s.Fatal("Physical keyboard has incorrect AT Raw Set 2 mode")
	}
}
