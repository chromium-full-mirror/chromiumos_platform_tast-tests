// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package virtualmultidisplay contains tests that are for multidisplay running on Betty devices (mainly to ensure the current virtual display system is working).
package virtualmultidisplay

import (
	"context"
	"time"

	virtualmultidisplay "go.chromium.org/tast-tests/cros/local/virtualmultidisplay"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Hotplug,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures that the multi display test fixture is capable of display hotplugging",
		Contacts:     []string{"arc-framework+tast@google.com", "brpol@google.com"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "virtual_multidisplay"},
		Timeout:      2 * time.Minute,
		Fixture:      virtualmultidisplay.VirtualMultiDisplay,
	})
}

// Hotplug tests that the multidisplay fixture works as intended and displays can be hotplugged.
func Hotplug(ctx context.Context, s *testing.State) {
	displayController := s.FixtValue().(virtualmultidisplay.HasVirtualDisplayController).VirtualDisplayController()

	internalDisplayID, err := displayController.InternalDisplayId()
	if err != nil {
		s.Fatal("Could not get internal display id: ", err)
	}

	externalDisplays, err := displayController.ExternalDisplayIds()
	if err != nil {
		s.Fatal("Could not get external display ids: ", err)
	}

	allDisplays := append([]int{internalDisplayID}, externalDisplays...)

	// Initial display is enabled.
	if enabled, err := displayController.DisplayEnabled(internalDisplayID); err != nil {
		s.Error("Could not check if display enabled: ", err)
	} else if !enabled {
		s.Error("Initial display not enabled")
	}

	for _, id := range allDisplays {
		if err := displayController.EnableDisplay(id); err != nil {
			s.Errorf("Could not enable display %d due to: %s", id, err)
		}

		if enabled, err := displayController.DisplayEnabled(id); err != nil {
			s.Errorf("Could not check if display %d enabled: %s", id, err)
		} else if !enabled {
			s.Errorf("Display %d not enabled", id)
		}
	}

	for _, id := range allDisplays {
		if err := displayController.DisableDisplay(id); err != nil {
			s.Errorf("Could not disable display %d due to: %s", id, err)
		}

		if enabled, err := displayController.DisplayEnabled(id); err != nil {
			s.Errorf("Could not check if display %d disabled: %s", id, err)
		} else if enabled {
			s.Errorf("Display %d enabled, but should be disabled", id)
		}
	}
}
