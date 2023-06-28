// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/ambient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mtbf/youtube"
	"go.chromium.org/tast-tests/cros/local/personalization"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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
		Attr:         []string{"group:mainline", "informational"},
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
	testParams := s.Param().(ambient.TestParams)
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	if err := uiauto.Combine("Open ambient subpage and enable screen saver",
		ambient.OpenAmbientSubpage(ui),
		ambient.EnableAmbientMode(ui),
		prepareScreenSaver(tconn, ui, testParams))(ctx); err != nil {
		s.Fatalf("Failed to prepare %v/%v screen saver: %v", testParams.TopicSource, testParams.Theme, err)
	}

	if testParams.PlayTestVideo {
		kb, err := input.VirtualKeyboard(ctx)
		if err != nil {
			s.Fatal("Failed to open the keyboard: ", err)
		}
		defer kb.Close(cleanupCtx)

		var uiHandler cuj.UIActionHandler
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
		defer uiHandler.Close(cleanupCtx)

		// Open up an arbitrary Youtube video to test "media string". The name of
		// the media playing should be displayed in the screen saver.
		const extendedDisplay = false
		videoApp := youtube.NewYtWeb(cr.Browser(), tconn, kb, extendedDisplay, ui, uiHandler)
		if err := videoApp.OpenAndPlayVideo(ambient.TestVideoSrc)(ctx); err != nil {
			s.Fatalf("Failed to open %s: %v", ambient.TestVideoSrc.URL, err)
		}
		defer videoApp.Close(cleanupCtx)
	}

	if err := uiauto.Combine("Run screen saver and unlock screen",
		ambient.TestLockScreenIdle(cr, tconn, ui, testParams),
		ambient.UnlockScreen(tconn, cr.Creds().User, cr.Creds().Pass))(ctx); err != nil {
		s.Fatalf("Failed to run %v/%v screen saver: %v", testParams.TopicSource, testParams.Theme, err)
	}
}

func prepareScreenSaver(tconn *chrome.TestConn, ui *uiauto.Context, testParams ambient.TestParams) uiauto.Action {
	return func(ctx context.Context) error {
		themeContainer := nodewith.Role(role.RadioButton).Name(testParams.Theme)
		if err := uiauto.Combine("Choose animation theme",
			ui.FocusAndWait(themeContainer),
			ui.LeftClick(themeContainer))(ctx); err != nil {
			return errors.Wrapf(err, "failed to select %v", testParams.Theme)
		}

		topicSourceContainer := nodewith.Role(role.RadioButton).NameContaining(testParams.TopicSource).Ancestor(nodewith.Attribute("description", "Image source"))
		albumsFinder := nodewith.Role(role.ListBoxOption).HasClass("album")

		if err := uiauto.Combine("Choose topic source",
			ui.FocusAndWait(topicSourceContainer),
			ui.LeftClick(topicSourceContainer),
			ui.WaitUntilExists(albumsFinder.First()))(ctx); err != nil {
			return errors.Wrapf(err, "failed to select %v", testParams.TopicSource)
		}

		albums, err := ui.NodesInfo(ctx, albumsFinder)
		if err != nil {
			return errors.Wrapf(err, "failed to find %v albums", testParams.TopicSource)
		}
		if len(albums) < 2 {
			return errors.Errorf("at least 2 %v albums expected", testParams.TopicSource)
		}

		// For animated themes, trust the default album selection. Test cases for
		// slideshow theme will verify that the default album selection is correct
		// and test custom album selection.
		if testParams.Theme == ambient.SlideShow {
			if testParams.TopicSource == ambient.GooglePhotos {
				var albumNames []string
				// Select all Google Photos albums.
				for i, album := range albums {
					if ambient.IsAlbumSelected(&album) {
						return errors.Errorf("Google Photos album %d should be unselected", i)
					}
					if err := ambient.SelectAlbum(ctx, ui, &album); err != nil {
						return errors.Wrapf(err, "failed to select Google Photos album %d", i)
					}
					albumNames = append(albumNames, album.Name)
				}
				expectedText := strings.Join(albumNames, " ")
				if err := waitForCurrentlySetWithName(ui, expectedText)(ctx); err != nil {
					return errors.Wrap(err, "failed to find matching currently set header")
				}
			} else if testParams.TopicSource == ambient.ArtGallery {
				// Turn off all but one art gallery album.
				for i, album := range albums[1:] {
					if !ambient.IsAlbumSelected(&album) {
						return errors.Errorf("Art album %d should be selected", i)
					}
					if err := ambient.DeselectAlbum(ctx, ui, &album); err != nil {
						return errors.Wrapf(err, "failed to deselect Art Gallery album %d", i)
					}
				}
				if err := waitForCurrentlySetWithName(ui, albums[0].Name)(ctx); err != nil {
					return errors.Wrap(err, "failed to wait for currently set")
				}
			} else {
				return errors.Errorf("topicSource - %v is invalid", testParams.TopicSource)
			}
		} else if testParams.Theme == ambient.VideoTheme {
			// Always make sure only the default video is selected initially.
			selectedVideos, err := ui.NodesInfo(ctx, nodewith.HasClass(ambient.AlbumSelectedClassName))
			if err != nil {
				return errors.Wrap(err, "failed to find the selected video albums")
			}
			if len(selectedVideos) != 1 {
				return errors.New("exactly 1 selected video album expected")
			}
			if selectedVideos[0].Name != ambient.DefaultVideoName {
				return errors.New("incorrect default ambient video is selected")
			}

			// Select the correct video if a non-default is requested.
			if testParams.VideoThemeAlbum != ambient.DefaultVideoName {
				albumToSelect := ambient.FindAlbumWithName(testParams.VideoThemeAlbum, albums)
				if albumToSelect == nil {
					return errors.New("failed to find video album with name " + testParams.VideoThemeAlbum)
				}
				if err := ambient.SelectAlbum(ctx, ui, albumToSelect); err != nil {
					return errors.Wrap(err, "failed to select video album "+testParams.VideoThemeAlbum)
				}
			}
		}

		// Close Personalization Hub after ambient mode setup is finished.
		if err := uiauto.Combine("Close personalization app and set device settings",
			personalization.ClosePersonalizationHub(ui),
			ambient.SetDeviceSettings(tconn, ambient.DeviceSettings{
				LockScreenIdle:         1 * time.Second,
				BackgroundLockScreen:   2 * time.Second,
				PhotoRefreshInterval:   1 * time.Second,
				AnimationPlaybackSpeed: testParams.AnimationPlaybackSpeed,
			}))(ctx); err != nil {
			return errors.Wrap(err, "failed to prepare screen saver")
		}

		return nil
	}
}

func waitForCurrentlySetWithName(ui *uiauto.Context, name string) uiauto.Action {
	currentlySetNode := nodewith.NameStartingWith(fmt.Sprintf("Currently set %v", name)).Role(role.Heading).Ancestor(personalization.PersonalizationHubWindow)
	return ui.WaitUntilExists(currentlySetNode)
}
