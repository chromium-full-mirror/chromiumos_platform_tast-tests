// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
)

var (
	extendedChamPath    = "extendedModeChameleonDisplay"
	extendedDisplayPath = "extendedModeInternalDisplay"
	mirrorChamPath      = "mirrorModeChameleonDisplay"
	mirrorDisplayPath   = "mirrorModeInternalDisplay"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChameleonSwitchMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Switch between extended and mirror mode and compare display screenshots",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		Attr: []string{
			"group:graphics",
			"graphics_chameleon_igt",
			"graphics_nightly",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"graphics.chameleon_ip"},
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{{
			Name: "extended_dp1",
			Val: graphics.ChameleonTest{
				Port:       "dp1",
				Iterations: 1,
				Display:    graphics.UIExtended,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "extended_dp2",
			Val: graphics.ChameleonTest{
				Port:       "dp2",
				Iterations: 1,
				Display:    graphics.UIExtended,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "extended_hdmi1",
			Val: graphics.ChameleonTest{
				Port:       "hdmi1",
				Iterations: 1,
				Display:    graphics.UIExtended,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "extended_hdmi2",
			Val: graphics.ChameleonTest{
				Port:       "hdmi2",
				Iterations: 1,
				Display:    graphics.UIExtended,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "mirror_dp1",
			Val: graphics.ChameleonTest{
				Port:       "dp1",
				Iterations: 1,
				Display:    graphics.UIMirror,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "mirror_dp2",
			Val: graphics.ChameleonTest{
				Port:       "dp2",
				Iterations: 1,
				Display:    graphics.UIMirror,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "mirror_hdmi1",
			Val: graphics.ChameleonTest{
				Port:       "hdmi1",
				Iterations: 1,
				Display:    graphics.UIMirror,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "mirror_hdmi2",
			Val: graphics.ChameleonTest{
				Port:       "hdmi2",
				Iterations: 1,
				Display:    graphics.UIMirror,
			},
			Timeout: chrome.LoginTimeout + time.Minute,
		}, {
			Name: "stress_dp1",
			Val: graphics.ChameleonTest{
				Port:       "dp1",
				Iterations: 3,
				Display:    graphics.AlternateExtendedMirror,
			},
			Timeout: chrome.LoginTimeout + 5*time.Minute,
		}, {
			Name: "stress_dp2",
			Val: graphics.ChameleonTest{
				Port:       "dp2",
				Iterations: 3,
				Display:    graphics.AlternateExtendedMirror,
			},
			Timeout: chrome.LoginTimeout + 5*time.Minute,
		}, {
			Name: "stress_hdmi1",
			Val: graphics.ChameleonTest{
				Port:       "hdmi1",
				Iterations: 3,
				Display:    graphics.AlternateExtendedMirror,
			},
			Timeout: chrome.LoginTimeout + 5*time.Minute,
		}, {
			Name: "stress_hdmi2",
			Val: graphics.ChameleonTest{
				Port:       "hdmi2",
				Iterations: 3,
				Display:    graphics.AlternateExtendedMirror,
			},
			Timeout: chrome.LoginTimeout + 5*time.Minute,
		}},
	})
}

func ChameleonSwitchMode(ctx context.Context, s *testing.State) {
	testOpt := s.Param().(graphics.ChameleonTest)
	portStr := testOpt.Port
	pixelDiffThreshold := 5000

	cham, err := graphics.ChameleonGetConnection(ctx)
	if err != nil {
		s.Fatal("Failed to get the Chameleond instance: ", err)
	}

	shouldUsePort, port, err := graphics.ChameleonShouldUsePort(ctx, cham, portStr)
	if err != nil {
		s.Fatalf("Failed to determine if plug can be used for port %s: %s", portStr, err)
	}
	if !shouldUsePort {
		s.Logf("Chameleon is not plugged into port %d", port)
		return
	}
	err = graphics.ChameleonPlug(ctx, cham, port)
	if err != nil {
		s.Fatalf("Failed to get stable video input from a physically plugged port %d: %s", port, err)
	}
	defer func(ctx context.Context) {
		err = cham.Unplug(ctx, port)
		if err != nil {
			s.Fatalf("Failed to unplug a physically plugged port %d: %s ", port, err)
		}
	}(ctx)

	// Log in to Chrome.
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)

	conn, err := cr.NewConn(ctx, "chrome://gpu")
	if err != nil {
		s.Fatal("Failed to load chrome://gpu: ", err)
	}
	defer conn.Close()

	// Switch between extended and mirror modes to take screenshots for comparison
	sameExtendedImage := false
	sameMirrorImage := false
	resWidth, resHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		s.Fatal("Failed to detect prelogin resolution: ", err)
	}

	for i := 0; i < testOpt.Iterations; i++ {
		if testOpt.Display == graphics.UIMirror || testOpt.Display == graphics.AlternateExtendedMirror {
			s.Log("Switching to mirror mode")
			sameMirrorImage, err = testMirrorModeMatch(ctx, cr, cham, port, s.OutDir(), i, pixelDiffThreshold)
			if err != nil {
				s.Fatal("Failed to checkMirrorModeDisplay: ", err)
			}

			if !sameMirrorImage {
				s.Fatal("Display did not show same screenshot in mirror mode")
			}

			err = assertSameResolution(ctx, cham, port, resWidth, resHeight)
			if err != nil {
				s.Fatal("Failed to compare resolutions after switching to mirror mode: ", err)
			}
		}

		if testOpt.Display == graphics.UIExtended || testOpt.Display == graphics.AlternateExtendedMirror {
			s.Log("Switching to extended mode")
			sameExtendedImage, err = testExtendedModeMatch(ctx, cr, cham, port, s.OutDir(), i, pixelDiffThreshold)
			if err != nil {
				s.Fatal("Failed to checkExtendedModeDisplay: ", err)
			}

			if !sameExtendedImage {
				s.Fatal("Display did not show same screenshot in extended mode")
			}

			err = assertSameResolution(ctx, cham, port, resWidth, resHeight)
			if err != nil {
				s.Fatal("Failed to compare resolutions after switching to extended mode: ", err)
			}
		}

		if testOpt.Display == graphics.AlternateExtendedMirror {
			mirrorDisplayPath := filepath.Join(s.OutDir(), mirrorDisplayPath+strconv.Itoa(i)+".png")
			extendedDisplayPath := filepath.Join(s.OutDir(), extendedDisplayPath+strconv.Itoa(i)+".png")

			// Comparing the ChromeOS desktop screenshots is racy since the desktop keeps changing.
			// Ideally, we put something fullscreen here. Ex. Animated desktop backgrounds and clock in UI.
			isInternalDisplaysSame, err := graphics.ChameleonPerceptualDiff(ctx, mirrorDisplayPath, extendedDisplayPath, s.OutDir(), pixelDiffThreshold)
			if err != nil {
				s.Fatal("Failed to compare internal display screenshots: ", err)
			}

			if !isInternalDisplaysSame {
				continue
			}

			if !sameMirrorImage || !sameExtendedImage {
				break
			}
		}
	}

	if testOpt.Display == graphics.AlternateExtendedMirror && (!sameMirrorImage || !sameExtendedImage) {
		s.Fatal("Display did not show same screenshot for mirror or external mode")
	}
}

// testMirrorModeMatch switches to mirror mode and takes a screenshot of the Chameleon and eDP.
// Then compares if they are the same screenshot
func testMirrorModeMatch(ctx context.Context, cr *chrome.Chrome, cham chameleon.Chameleond, port chameleon.PortID, outDir string, idx, pixelDiffThreshold int) (bool, error) {
	err := graphics.SwitchDisplayMode(ctx, cr, true)
	if err != nil {
		return false, errors.Errorf("failed to switch to mirror mode: %s", err)
	}

	curDisplayPath := filepath.Join(outDir, mirrorDisplayPath+strconv.Itoa(idx)+".png")
	curChamPath := filepath.Join(outDir, mirrorChamPath+strconv.Itoa(idx)+".png")

	// TODO(b:263163784): Replace CaptureChrome and ChameleonGetScreenshot with screen-util-tools command.
	err = screenshot.CaptureChrome(ctx, cr, curDisplayPath)
	if err != nil {
		return false, errors.Errorf("failed to screenshot eDP: %s", err)
	}
	err = graphics.ChameleonGetScreenshot(ctx, cham, port, curChamPath)
	if err != nil {
		return false, errors.Errorf("cannot get Chameleon screenshot: %s", err)
	}

	isChamAndDisplaySame, err := graphics.ChameleonPerceptualDiff(ctx, curChamPath, curDisplayPath, outDir, pixelDiffThreshold)
	if err != nil {
		return false, errors.Errorf("failed to compare internal display screenshots: %s", err)
	}

	return isChamAndDisplaySame, nil
}

// testExtendedModeMatch switches to extended mode and takes a screenshot of the Chameleon.
// Then compares if they are the previous screenshot is the same as the current screenshot.
func testExtendedModeMatch(ctx context.Context, cr *chrome.Chrome, cham chameleon.Chameleond, port chameleon.PortID, outDir string, idx, pixelDiffThreshold int) (bool, error) {
	err := graphics.SwitchDisplayMode(ctx, cr, false)
	if err != nil {
		return false, errors.Errorf("failed to switch to extended mode: %s", err)
	}

	curDisplayPath := filepath.Join(outDir, extendedDisplayPath+strconv.Itoa(idx)+".png")
	curChamPath := filepath.Join(outDir, extendedChamPath+strconv.Itoa(idx)+".png")

	// TODO(b:263163784): Replace CaptureChrome and ChameleonGetScreenshot with screen-util-tools command.
	err = screenshot.CaptureChrome(ctx, cr, curDisplayPath)
	if err != nil {
		return false, errors.Errorf("failed to screenshot eDP: %s", err)
	}
	err = graphics.ChameleonGetScreenshot(ctx, cham, port, curChamPath)
	if err != nil {
		return false, errors.Errorf("cannot get Chameleon screenshot: %s", err)
	}

	isChamAndDisplaySame, err := graphics.ChameleonPerceptualDiff(ctx, curChamPath, curDisplayPath, outDir, pixelDiffThreshold)
	if err != nil {
		return false, errors.Errorf("failed to compare display screenshots: %s", err)
	}

	return !isChamAndDisplaySame, nil
}

// assertSameResolution asserts that the previous resolution is the same as the current resolution
func assertSameResolution(ctx context.Context, cham chameleon.Chameleond, port chameleon.PortID, resWidth, resHeight int) error {
	curWidth, curHeight, err := cham.DetectResolution(ctx, port)
	if err != nil {
		return errors.Errorf("failed to detect resolution: %s", err)
	}

	if resWidth != curWidth && resHeight != curHeight {
		return errors.Errorf("At port %d, %dx%d not equal to %dx%d", port, resWidth, resHeight, curWidth, curHeight)
	}
	return nil
}
