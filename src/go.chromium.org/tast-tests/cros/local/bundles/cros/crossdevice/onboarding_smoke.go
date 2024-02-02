// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crossdevice

import (
	"context"
	"time"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OnboardingSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Runs the cross device onboarding fixture to prevent its failures from failing other tests in the cross device suite",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"bhartmire@google.com",
		},
		BugComponent: "b:1108889",
		Attr:         []string{"group:cross-device", "cross-device_crossdevice"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:   "crossdeviceOnboardedAllFeatures",
				ExtraAttr: []string{"cross-device_cq"},
			},
			{
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedAllFeaturesFloss",
				ExtraAttr: []string{"cross-device_floss"},
			},
		},
	})
}

func OnboardingSmoke(ctx context.Context, s *testing.State) {}
