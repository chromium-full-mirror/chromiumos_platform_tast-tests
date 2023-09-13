// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetScreenSaver,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test setting screen saver options and starting screen saver",
		Contacts: []string{
			"assistive-eng@google.com",
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-e92e2d70-5969-4405-9cdd-c3ecee573f81",
		}},
		VarDeps:      []string{"ambient.username", "ambient.password"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      "personalizationScreenSaverClamshell",
		Params: []testing.Param{
			{
				Name: "google_photos",
				Val: ambient.TestParams{
					TopicSource:    ambient.GooglePhotos,
					Theme:          ambient.SlideShow,
					StartupTimeout: ambient.StartSlideShowDefaultTimeout,
				},
			},
			{
				Name: "art_gallery",
				Val: ambient.TestParams{
					TopicSource:    ambient.ArtGallery,
					Theme:          ambient.SlideShow,
					StartupTimeout: ambient.StartSlideShowDefaultTimeout,
				},
			},
			// For animated themes:
			// * Their image handling is agnostic to the topic source, so it would be
			//   a waste of test time/resources to test all topic sources for each
			//   theme.
			// * ArtGallery is chosen as the topic source since some of the photos may
			//   have attribution text (whereas personal photos do not). This gives
			//   the test better coverage since attribution text handling is not
			//   trivial.
			{
				Name: "feel_the_breeze",
				Val: ambient.TestParams{
					TopicSource:            ambient.ArtGallery,
					Theme:                  ambient.FeelTheBreeze,
					AnimationPlaybackSpeed: ambient.AnimationFastForwardPlaybackSpeed,
					StartupTimeout:         ambient.StartAnimationDefaultTimeout,
				},
			},
			{
				Name: "float_on_by",
				Val: ambient.TestParams{
					TopicSource:            ambient.ArtGallery,
					Theme:                  ambient.FloatOnBy,
					AnimationPlaybackSpeed: ambient.AnimationFastForwardPlaybackSpeed,
					StartupTimeout:         ambient.StartAnimationDefaultTimeout,
					PlayTestVideo:          true,
				},
			},
		},
	})
}

func SetScreenSaver(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	testParams := s.Param().(ambient.TestParams)
	if err := ambient.SetScreenSaverHelper(ctx, cr, testParams, s.OutDir(), s.HasError); err != nil {
		s.Fatal("Fail to set screen saver: ", err)
	}
}
