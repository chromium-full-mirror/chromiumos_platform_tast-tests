// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/youtube"
	"go.chromium.org/tast-tests/cros/local/arc/apputil/youtubemusic"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

const testfile = "fivemin_audio.mp3"

func init() {
	testing.AddTest(&testing.Test{
		Func:         MediaSourceUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check if the media control widget is displaying correct media source",
		Contacts: []string{
			"cros-arc-te@google.com",
			"cienet-development@googlegroups.com",
			"cj.tsai@cienet.com",
		},
		BugComponent: "b:1052117", // ChromeOS > Software > ARC++ > EngProd
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc"},
		Data:         []string{testfile},
		// There are two apps to be installed in this case.
		Timeout: 2*time.Minute + 2*apputil.InstallationTimeout,
		Fixture: "arcBootedWithPlayStore",
	})
}

// MediaSourceUI checks the media control is displaying the most recent activated media source.
func MediaSourceUI(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC
	device := s.FixtValue().(*arc.PreData).UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create the keyboard: ", err)
	}
	defer kb.Close(ctx)

	s.Log("Copy audio file")

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	audioFileLocation := filepath.Join(downloadsPath, testfile)
	if _, err := os.Stat(audioFileLocation); os.IsNotExist(err) {
		if err := fsutil.CopyFile(s.DataPath(testfile), audioFileLocation); err != nil {
			s.Fatal("Failed to copy file: ", err)
		}
		defer os.Remove(audioFileLocation)
	} else {
		s.Fatal("Failed to get audio file info: ", err)
	}

	ytmusicVideo := "Beat It (Official 4K Video)"
	ytappLink := "https://www.youtube.com/watch?v=JE3-LkMqBfM"
	ytappVideo := "Whale Songs and AI, for everyone to explore"

	for appName, media := range map[string]*apputil.Media{
		apps.Gallery.Name:    apputil.NewMedia("", testfile),
		youtubemusic.AppName: apputil.NewMedia(ytmusicVideo, ytmusicVideo),
		youtube.AppName:      apputil.NewMedia(ytappLink, ytappVideo),
	} {
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			var err error
			var app apputil.ARCMediaPlayer
			switch appName {
			case youtube.AppName:
				app, err = youtube.NewApp(ctx, kb, tconn, a, device)
			case apps.Gallery.Name:
				app = newGallery(ctx, tconn, cr, filepath.Join(s.OutDir(), appName))
			case youtubemusic.AppName:
				app, err = newYtMusic(ctx, kb, tconn, a, device)
			default:
				s.Fatal("Failed to create media app instance: unexpected media source: ", appName)
			}
			if err != nil {
				s.Fatalf("Failed to create media app instance for %q: %v", appName, err)
			}

			if err := app.Install(ctx); err != nil {
				s.Fatalf("Failed to install app %q: %v", appName, err)
			}

			outDir := filepath.Join(s.OutDir(), appName)
			if _, err := app.Launch(ctx); err != nil {
				s.Fatalf("Failed to launch app %q: %v", appName, err)
			}
			defer app.Close(cleanupCtx, cr, s.HasError, outDir)
			defer faillog.DumpUITreeOnErrorToFile(cleanupCtx, outDir, s.HasError, tconn, "ui_unpin.txt")

			if err := app.Play(ctx, media); err != nil {
				s.Fatalf("Failed to play media %q in app %q: %v", media.Subtitle, appName, err)
			}

			ui := uiauto.New(tconn)
			// Unpin media pod if it is pinned by default.
			if err := ui.WaitUntilExists(quicksettings.PinnedMediaControls)(ctx); err == nil {
				if err := quicksettings.UnpinMediaControlsPod(tconn)(ctx); err != nil {
					s.Fatal("Failed to unpin media control pod: ", err)
				}
			}

			if err := quicksettings.Show(ctx, tconn); err != nil {
				s.Fatal("Failed to show quicksettings: ", err)
			}
			defer quicksettings.Hide(cleanupCtx, tconn)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, outDir, s.HasError, cr, "ui_quicksettings")

			mediaControlsPod := quicksettings.MediaControlsPod()
			resourceName := nodewith.Name(media.Subtitle).HasClass("Label").Ancestor(mediaControlsPod)
			if err := ui.WaitUntilExists(resourceName)(ctx); err != nil {
				s.Fatal("Failed to check media control UI: ", err)
			}
		}

		s.Run(ctx, appName, f)
	}
}

// ytMusic represents the media app: YouTube Music.
type ytMusic struct {
	*youtubemusic.YouTubeMusic
}

// ytMusic is built to override the original play() function to play a video
// and conform to ARCMediaPlayer interface.
var _ apputil.ARCMediaPlayer = (*ytMusic)(nil)

// newYtMusic returns ytMusic instance.
func newYtMusic(ctx context.Context, kb *input.KeyboardEventWriter, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device) (*ytMusic, error) {
	ytm, err := youtubemusic.New(ctx, kb, tconn, a, d)
	return &ytMusic{ytm}, err
}

// Play searches the specified media source and plays it by YouTube Music.
func (ytm *ytMusic) Play(ctx context.Context, media *apputil.Media) error {
	// YouTube Music needs to play a video in this test case.
	return ytm.PlayVideo(ctx, media)
}

// gallery represents the media app: Gallery.
type gallery struct {
	tconn *chrome.TestConn
	// cr and outDir are for fail-log usage.
	cr     *chrome.Chrome
	outDir string
}

// gallery is built to conform to ARCMediaPlayer interface.
var _ apputil.ARCMediaPlayer = (*gallery)(nil)

// newGallery returns gallery instance.
func newGallery(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, outDir string) *gallery {
	// cr and outDir are for fail-log usage.
	return &gallery{tconn: tconn, cr: cr, outDir: outDir}
}

// Install isntalls the Gallery.
func (g *gallery) Install(ctx context.Context) error {
	// Gallery does not need to install in advance.
	return nil
}

// Launch launches the Gallery.
func (g *gallery) Launch(ctx context.Context) (time.Duration, error) {
	// Gallery does not need to launch in advance.
	// It will open by clicking on the file in the Play method.
	return 0, nil
}

// Play searches the specified media source and plays it by Gallery.
func (g *gallery) Play(ctx context.Context, media *apputil.Media) (retErr error) {
	files, err := filesapp.Launch(ctx, g.tconn)
	if err != nil {
		return err
	}
	defer files.Close(ctx)
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, g.outDir, func() bool { return retErr != nil }, g.cr, "ui_filesapp")

	gallery := nodewith.NameStartingWith(apps.Gallery.Name).HasClass("BrowserFrame")
	return uiauto.Combine("play from files app",
		files.OpenDownloads(),
		files.OpenFile(media.Subtitle),
		uiauto.New(g.tconn).WaitUntilExists(gallery),
	)(ctx)
}

// Close closes gallery and files app.
func (g *gallery) Close(ctx context.Context, cr *chrome.Chrome, hasErr func() bool, dumpDir string) error {
	faillog.DumpUITreeWithScreenshotOnError(ctx, dumpDir, hasErr, cr, "ui_gallery")
	if err := apps.Close(ctx, g.tconn, apps.Gallery.ID); err != nil {
		testing.ContextLog(ctx, "Failed to close gallery")
	}
	return nil
}
