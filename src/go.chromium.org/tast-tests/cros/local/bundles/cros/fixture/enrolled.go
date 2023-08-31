// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Enrolled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Indicator test for the enrolled fixture",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
			"gabormagda@google.com", // Test author
		},
		BugComponent: "b:1111632", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Enrollment
		Params: []testing.Param{{
			Name:      "golden",
			ExtraAttr: []string{"group:golden_tier"},
		}, {
			Name:      "medium",
			ExtraAttr: []string{"group:medium_low_tier"},
		}, {
			Name:      "hardware",
			ExtraAttr: []string{"group:hardware"},
		}, {
			Name:      "complementary",
			ExtraAttr: []string{"group:complementary"},
		}},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.Enrolled,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Enroll an unmanaged device by entering credentials manually to
			// ensure it enrolls with user interaction.
			// COM_FOUND_CUJ26_TASK3_WF1
			Value: "screenplay-e3feb0c8-a73b-4974-acf6-310348498e62",
		}},
	})
}

func Enrolled(ctx context.Context, s *testing.State) {
	// The test should always pass.
	// When this test fails, it indicates a fixture failure.
}
