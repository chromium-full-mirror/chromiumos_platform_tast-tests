// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	ambientCommon "go.chromium.org/tast-tests/cros/common/ambient"
	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SetScreenSaver,
		Desc: "Test setting screen saver options and starting screen saver",
		Contacts: []string{
			"cros-p13n-eng@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr: []string{
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-e92e2d70-5969-4405-9cdd-c3ecee573f81",
		}},
		VarDeps:      []string{ambientCommon.AccountVarName},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      5 * time.Minute,
		Fixture:      personalization.GooglePhotosClamshellFixture,
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

					// TODO(b/406780009): enable PlayTestVideo to true once bug is resolved.
					PlayTestVideo: false,
				},
			},
		},
	})
}

func SetScreenSaver(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Fail to get test api conn: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	testParams := s.Param().(ambient.TestParams)
	if err := ambient.SetScreenSaverHelper(ctx, cr, tconn, testParams); err != nil {
		s.Fatal("Fail to set screen saver: ", err)
	}
}
