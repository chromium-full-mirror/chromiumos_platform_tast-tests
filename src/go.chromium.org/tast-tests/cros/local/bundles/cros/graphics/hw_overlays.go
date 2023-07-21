// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/graphics"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	binArraySize     = 16
	canvas2DFile     = "canvas_2d_low_latency.html"
	canvas3DFile     = "canvas_3d.html"
	dri0Path         = "/sys/kernel/debug/dri/0/state"
	dri1Path         = "/sys/kernel/debug/dri/1/state"
	dri2Path         = "/sys/kernel/debug/dri/2/state"
	minNumOfPlanes   = 3
	minNumOverlays   = 1 // One since after a test is ran an Overlay becomes freed up.
	minimumDrawCalls = 5
	videoFile        = "video.html"
	videoMedia       = "traffic-1920x1080.vp9.webm"
	webpageRunTime   = 30 * time.Second
)

var (
	crtcDisabledPattern   = regexp.MustCompile(`\s+crtc-pos=0x0`)
	crtcPosAndSizePattern = regexp.MustCompile(`\s+crtc-pos=\d+x\d+`)
)

type pageTestParams struct {
	browserType browser.Type
	file        string
	media       string
	title       string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HwOverlays,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "This test runs HTML files and monitors the behavior of hardware overlays",
		Contacts: []string{
			"chromeos-gfx@chromium.org",
			"syedfaaiz@google.com",
		},
		BugComponent: "b:995569",
		Attr:         []string{"graphics_perbuild", "group:graphics"},
		// Atomic, Chrome and Internal displays are essentials for this test.
		SoftwareDeps: []string{"drm_atomic", "chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.SupportsHardwareOverlays()),
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Name:      "canvas_2d",
			Fixture:   "chromeGraphicsHwOverlays",
			ExtraData: []string{canvas2DFile},
			Val: pageTestParams{
				browserType: browser.TypeAsh,
				file:        canvas2DFile,
				title:       "Canvas 2D Low Latency",
			},
		}, {
			Name:      "canvas_3d",
			Fixture:   "chromeGraphicsHwOverlays",
			ExtraData: []string{canvas3DFile},
			Val: pageTestParams{
				browserType: browser.TypeAsh,
				file:        canvas3DFile,
				title:       "Canvas 3D",
			},
		}, {
			Name:      "video",
			Fixture:   "chromeGraphicsHwOverlays",
			ExtraData: []string{videoFile, videoMedia},
			// Video test requires NV12 overlay support.
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			Val: pageTestParams{
				browserType: browser.TypeAsh,
				file:        videoFile,
				media:       videoMedia,
				title:       "Video playback",
			},
		}, {
			Name:              "canvas_2d_lacros",
			Fixture:           "chromeGraphicsHwOverlaysLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraData:         []string{canvas2DFile},
			Val: pageTestParams{
				browserType: browser.TypeLacros,
				file:        canvas2DFile,
				title:       "Canvas 2D Low Latency",
			},
		}, {
			Name:              "canvas_3d_lacros",
			Fixture:           "chromeGraphicsHwOverlaysLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraData:         []string{canvas3DFile},
			Val: pageTestParams{
				browserType: browser.TypeLacros,
				file:        canvas3DFile,
				title:       "Canvas 3D",
			},
		}, {
			Name:              "video_lacros",
			Fixture:           "chromeGraphicsHwOverlaysLacros",
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraData:         []string{videoFile, videoMedia},
			// Video test requires NV12 overlay support.
			ExtraHardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
			Val: pageTestParams{
				browserType: browser.TypeLacros,
				file:        videoFile,
				media:       videoMedia,
				title:       "Video playback",
			},
		}, {
			Name:              "canvas_2d_vulkan",
			Fixture:           "chromeGraphicsVulkan",
			ExtraSoftwareDeps: []string{"vulkan_composite"},
			ExtraData: []string{canvas2DFile},
			Val: pageTestParams{
				browserType: browser.TypeAsh,
				file:        canvas2DFile,
				title:       "Canvas 2D Low Latency",
			},
		}, {
			Name:              "canvas_3d_vulkan",
			Fixture:           "chromeGraphicsVulkan",
			ExtraSoftwareDeps: []string{"vulkan_composite"},
			ExtraData: []string{canvas3DFile},
			Val: pageTestParams{
				browserType: browser.TypeAsh,
				file:        canvas3DFile,
				title:       "Canvas 3D",
			},
		}, {
			Name:              "video_vulkan",
			Fixture:           "chromeGraphicsVulkan",
			ExtraSoftwareDeps: []string{"vulkan_composite"},
			ExtraData: []string{videoFile, videoMedia},
			Val: pageTestParams{
				browserType: browser.TypeAsh,
				file:        videoFile,
				media:       videoMedia,
				title:       "Video playback",
			},
		}},
	})
}

// checkIfNumPlanesGreaterThan takes a 2D array of 16 bit of plane number and accumulates votes per plane.
func checkIfNumPlanesGreaterThan(ctx context.Context, planes [][]uint64, minimumPlanes int) bool {
	var sum [binArraySize]uint64
	for row := 0; row < len(planes); row++ {
		for col := 0; col < len(planes[0]); col++ {
			sum[col] += planes[row][col]
		}
	}
	for _, numOfPlanes := range sum {
		if numOfPlanes >= uint64(minimumPlanes) {
			return true
		}
	}
	return false
}

// checkDriFileExists check if a file exists.
func checkDriFileExists(fileName string) bool {
	info, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// checkNumOverlays reads the kernel to figure out how many overlays are in use.
func checkNumOverlays(ctx context.Context) error {
	var filePath string
	if checkDriFileExists(dri0Path) {
		filePath = dri0Path
	} else if checkDriFileExists(dri1Path) {
		filePath = dri1Path
	} else if checkDriFileExists(dri2Path) {
		filePath = dri2Path
	} else {
		return errors.New("no dri debug file exists")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return errors.New("could not read dri debug file")
	}
	var count int
	for _, plane := range strings.Split(string(data), "\n") {
		if len(crtcDisabledPattern.FindStringSubmatch(plane)) > 0 {
			continue
		}
		if len(crtcPosAndSizePattern.FindStringSubmatch(plane)) > 0 {
			count++
		}
	}
	if count < minNumOverlays {
		return errors.New("Number of active overlays is less than the minimum needed")
	}
	return nil
}

func HwOverlays(ctx context.Context, s *testing.State) {
	// Check if we have the minimum required number of overlays through "modetest -p".
	out, err := graphics.GetModeTestPlanes(ctx)
	if err != nil {
		s.Fatalf("Failed to get number of modetest planes: %s", err)
	}
	if ans := checkIfNumPlanesGreaterThan(ctx, out, minNumOfPlanes); ans == false {
		s.Fatalf("Need at least %v planes to run the test on this device", minNumOfPlanes)
	}
	params := s.Param().(pageTestParams)
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	cr := s.FixtValue().(*chrome.Chrome)
	url := path.Join(server.URL, params.file)
	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(
		ctx, s.FixtValue().(chrome.HasChrome).Chrome(), s.Param().(pageTestParams).browserType, url)
	if err != nil {
		s.Fatalf("Failed to open %v: %v", url, err)
	}
	defer closeBrowser(ctx)
	defer conn.Close()
	ctconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	// If the test has specified a media to be loaded, load it now.
	if len(params.media) > 0 {
		if err = conn.Call(ctx, nil, "load_data_url", params.media); err != nil {
			s.Fatalf("Page failed to load url : %s: %s ", params.media, err)
		}
	}
	_, err = ash.WaitForAnyWindowWithTitle(ctx, ctconn, params.title)
	if err != nil {
		s.Fatalf("Failed to find the window that contains the %s app: %s", params.title, err)
	}
	if err = conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		s.Fatal("Failed to get the page ready: ", err)
	}
	defer ash.CloseAllWindows(ctx, ctconn)
	// GoBigSleepLint: Sleep is required to allow the webpage time to accumulate draw calls.
	testing.Sleep(ctx, webpageRunTime)
	var drawCounts int
	if err = conn.Eval(ctx, "parseInt(get_draw_passes_count())", &drawCounts); err != nil {
		s.Fatalf("Page failed to execute func get_draw_passes_count %s ", err)
	}
	if minimumDrawCalls > drawCounts {
		s.Fatalf("Total draw calls: %v are less than minimum acceptable draw calls : %v", drawCounts, minimumDrawCalls)
	}
	if err = checkNumOverlays(ctx); err != nil {
		s.Fatal("Verifying minimum overlays failed with: ", err)
	}
}
