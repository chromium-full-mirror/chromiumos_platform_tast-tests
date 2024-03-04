// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/audio"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioValidity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Audio validity test for arc",
		Contacts: []string{
			"chromeos-audio-bugs@google.com", // Media team
			"cychiang@chromium.org",          // Media team
			"paulhsia@chromium.org",          // Media team
			"judyhsiao@chromium.org",         // Author
		},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Audio
		BugComponent: "b:879188",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "group:audio", "group:cq-medium"},
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "playback",
				Val: audio.TestParameters{
					Class: "org.chromium.arc.testapp.arcaudiotest.TestOutputActivity",
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Fixture:           "arcBooted",
			},
			{
				Name: "playback_vm",
				Val: audio.TestParameters{
					Class: "org.chromium.arc.testapp.arcaudiotest.TestOutputActivity",
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				Fixture:           "arcBooted",
			},
			{
				Name: "record",
				Val: audio.TestParameters{
					Permission: "android.permission.RECORD_AUDIO",
					Class:      "org.chromium.arc.testapp.arcaudiotest.TestInputActivity",
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Fixture:           "arcBooted",
			},
			{
				Name: "record_vm",
				Val: audio.TestParameters{
					Permission: "android.permission.RECORD_AUDIO",
					Class:      "org.chromium.arc.testapp.arcaudiotest.TestInputActivity",
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				Fixture:           "arcBooted",
			},

			// Field-trial config off
			{
				Name: "playback_fieldtrial_testing_config_off",
				Val: audio.TestParameters{
					Class: "org.chromium.arc.testapp.arcaudiotest.TestOutputActivity",
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Fixture:           "arcBootedWithFieldTrialConfigOff",
			},
			{
				Name: "playback_vm_fieldtrial_testing_config_off",
				Val: audio.TestParameters{
					Class: "org.chromium.arc.testapp.arcaudiotest.TestOutputActivity",
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				Fixture:           "arcBootedWithFieldTrialConfigOff",
			},
			{
				Name: "record_fieldtrial_testing_config_off",
				Val: audio.TestParameters{
					Permission: "android.permission.RECORD_AUDIO",
					Class:      "org.chromium.arc.testapp.arcaudiotest.TestInputActivity",
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Fixture:           "arcBootedWithFieldTrialConfigOff",
			},
			{
				Name: "record_vm_fieldtrial_testing_config_off",
				Val: audio.TestParameters{
					Permission: "android.permission.RECORD_AUDIO",
					Class:      "org.chromium.arc.testapp.arcaudiotest.TestInputActivity",
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				Fixture:           "arcBootedWithFieldTrialConfigOff",
			},

			// Field-trial config on
			{
				Name: "playback_fieldtrial_testing_config_on",
				Val: audio.TestParameters{
					Class: "org.chromium.arc.testapp.arcaudiotest.TestOutputActivity",
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Fixture:           "arcBootedWithFieldTrialConfigOn",
			},
			{
				Name: "playback_vm_fieldtrial_testing_config_on",
				Val: audio.TestParameters{
					Class: "org.chromium.arc.testapp.arcaudiotest.TestOutputActivity",
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				Fixture:           "arcBootedWithFieldTrialConfigOn",
			},
			{
				Name: "record_fieldtrial_testing_config_on",
				Val: audio.TestParameters{
					Permission: "android.permission.RECORD_AUDIO",
					Class:      "org.chromium.arc.testapp.arcaudiotest.TestInputActivity",
				},
				ExtraSoftwareDeps: []string{"android_container"},
				Fixture:           "arcBootedWithFieldTrialConfigOn",
			},
			{
				Name: "record_vm_fieldtrial_testing_config_on",
				Val: audio.TestParameters{
					Permission: "android.permission.RECORD_AUDIO",
					Class:      "org.chromium.arc.testapp.arcaudiotest.TestInputActivity",
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				Fixture:           "arcBootedWithFieldTrialConfigOn",
			},
		},
	})
}

// AudioValidity runs audio validity tests.
func AudioValidity(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice
	param := s.Param().(audio.TestParameters)
	atast, err := audio.NewARCAudioTast(ctx, a, cr, d)
	if err != nil {
		s.Fatal("Failed to NewARCAudioTast: ", err)
	}
	if err := atast.RunAppTest(ctx, arc.APKPath(audio.Apk), param); err != nil {
		s.Error("Test failed: ", err)
	}
}
