// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetTimeOfDayScreenSaver,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting screen saver video and starting playback",
		Contacts: []string{
			"cros-personalization@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:cbx", "cbx_feature_enabled", "cbx_stable"},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		VarDeps:      []string{"ambient.username", "ambient.password"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      personalization.GooglePhotosClamshellFixture,
		Params: []testing.Param{
			{
				Name: "video_new_mexico",
				Val: ambient.TestParams{
					TopicSource:     ambient.VideoSource,
					Theme:           ambient.VideoTheme,
					VideoThemeAlbum: ambient.NewMexicoVideoName,
					StartupTimeout:  ambient.StartVideoDefaultTimeout,
				},
			},
			{
				Name: "video_clouds",
				Val: ambient.TestParams{
					TopicSource:     ambient.VideoSource,
					Theme:           ambient.VideoTheme,
					VideoThemeAlbum: ambient.CloudsVideoName,
					StartupTimeout:  ambient.StartVideoDefaultTimeout,
				},
			},
		},
	})
}

func SetTimeOfDayScreenSaver(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testParams := s.Param().(ambient.TestParams)
	if err := ambient.SetScreenSaverHelper(ctx, cr, testParams, s.OutDir(), s.HasError); err != nil {
		s.Fatal("Fail to set cbx screen saver: ", err)
	}
}
