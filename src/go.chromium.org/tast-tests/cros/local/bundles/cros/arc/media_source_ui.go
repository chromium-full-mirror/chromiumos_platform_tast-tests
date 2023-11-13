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
	"go.chromium.org/tast-tests/cros/local/arc/apputil/vlc"
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

const (
	testVideoFile     = "cars_144_h264.mp4"
	testAudiofile     = "fivemin_audio.mp3"
	testVideoSubtitle = "cars_144"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           MediaSourceUI,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Check if the media control widget is displaying correct media source",
		Contacts: []string{
			// "cros-arc-te@google.com",
			"cj.tsai@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1052117", // ChromeOS > Software > ARC++ > EngProd
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "gaia"},
		Data:         []string{testVideoFile, testAudiofile},
		// There are two apps to be installed in this case.
		Timeout: 2*time.Minute + 2*apputil.InstallationTimeout,
		Fixture: "arcBooted",
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

	res := &vlcAppTestResources{
		chrome: cr,
		kb:     kb,
		tconn:  tconn,
		arc:    a,
		device: device,
	}

	s.Log("Copy test file")

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}

	// Copy test files to Downloads directory.
	for _, file := range []string{testVideoFile, testAudiofile} {
		fileLocation := filepath.Join(downloadsPath, file)
		if _, err := os.Stat(fileLocation); os.IsNotExist(err) {
			if err := fsutil.CopyFile(s.DataPath(file), fileLocation); err != nil {
				s.Fatal("Failed to copy file: ", err)
			}
			defer os.Remove(fileLocation)
		} else if err != nil {
			s.Fatal("Failed to get test file info: ", err)
		}
	}
	for appName, media := range map[string]*apputil.Media{
		apps.Gallery.Name: apputil.NewMedia("", testAudiofile),
		vlc.AppName:       apputil.NewMedia(testVideoFile, testVideoSubtitle),
	} {
		f := func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			var err error
			var app apputil.ARCMediaPlayer
			switch appName {
			case apps.Gallery.Name:
				app = newGallery(ctx, tconn, cr, filepath.Join(s.OutDir(), appName))
			case vlc.AppName:
				app = newVLCPlayer(ctx, res)
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

// vlcPlayer represents the media app: VLC.
type vlcPlayer struct {
	*vlc.Vlc
	res *vlcAppTestResources
}

// vlcAppTestResources represents the resources needed for ARC app test.
type vlcAppTestResources struct {
	chrome *chrome.Chrome
	kb     *input.KeyboardEventWriter
	tconn  *chrome.TestConn
	arc    *arc.ARC
	device *ui.Device
}

// vlcPlayer is built to conform to ARCMediaPlayer interface.
var _ apputil.ARCMediaPlayer = (*vlcPlayer)(nil)

// newVLCPlayer returns vlcPlayer instance.
func newVLCPlayer(ctx context.Context, resources *vlcAppTestResources) *vlcPlayer {
	return &vlcPlayer{
		res: resources,
	}
}

// Play searches the specified media source and plays it by VLCPlayer.
func (vp *vlcPlayer) Play(ctx context.Context, media *apputil.Media) error {
	if err := vp.EnterDownloadFolder(ctx); err != nil {
		return err
	}
	return vp.Vlc.Play(ctx, &vlc.MediaInfo{
		FileName: media.Query,
		FileType: vlc.Video,
	})
}

// Install installs the VLCPlayer.
func (vp *vlcPlayer) Install(ctx context.Context) error {
	var err error
	if vp.Vlc, err = vlc.NewVLCPlayer(ctx, vp.res.chrome, vp.res.kb, vp.res.tconn, vp.res.arc, vp.res.device); err != nil {
		return err
	}
	return nil
}

// Launch launches the VLCPlayer.
func (vp *vlcPlayer) Launch(ctx context.Context) (time.Duration, error) {
	return 0, vp.Vlc.Launch(ctx)
}
