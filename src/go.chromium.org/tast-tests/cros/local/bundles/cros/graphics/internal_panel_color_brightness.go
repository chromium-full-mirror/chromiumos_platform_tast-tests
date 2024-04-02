// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

/*
 * Why this test?
 *
 * We need a validation that the internal display is functional. Best way to do that is to check if the screen is emitting a colored light that we can detect using the camera.
 * Change of colors is easier to detect than a white bright light as the ambient light might interfere with the measurements, and the camera processing might adjust the color brightness.
 * We chose green, because among the RGB, we have red and blue chromebooks.
 *
 * Full analysis: b/327458304
 */

func init() {
	testing.AddTest(&testing.Test{
		Func: InternalPanelColorBrightness,
		Desc: "Checks that the internal panel is emitting a colored light that we can detect using the camera to validate that Chrome appears on the screen",
		Contacts: []string{
			"chromeos-gfx-display@google.com",
			"markyacoub@google.com",
		},
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		Attr:         []string{"group:graphics", "graphics_nightly"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "gpuWatchHangs",
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func InternalPanelColorBrightness(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(ctx)

	// Render a green window to run the tests on it.
	if err := openGreenFullScreen(ctx, cr); err != nil {
		s.Fatal("Failed to open a green screen in fullscreen: ", err)
	}

	// Get measurements when the screen is off and compare when the screen is supposedly on.
	if err := graphics.SetSystemBrightness(ctx, 0.0); err != nil {
		s.Fatal("Failed to turn the brightness down: ", err)
	}

	darkImgPath := s.OutDir() + "/dark_shot.jpg"
	if err := takeCameraShot(ctx, cr, darkImgPath); err != nil {
		s.Fatal("Failed to take a dark camera shot: ", err)
	}
	ratioOff := getGreenToOtherColorRatio(ctx, darkImgPath, s)

	// Now compare when the screen is supposedly on to check if the brightness is actually changing.
	if err := graphics.SetSystemBrightness(ctx, 100.0); err != nil {
		s.Fatal("Failed to turn the brightness up: ", err)
	}

	brightImgPath := s.OutDir() + "/bright_shot.jpg"
	if err := takeCameraShot(ctx, cr, brightImgPath); err != nil {
		s.Fatal("Failed to take a bright camera shot: ", err)
	}
	ratioGreen := getGreenToOtherColorRatio(ctx, brightImgPath, s)
	s.Log("The screen brightness is changing. The ratio when the screen is off is ", ratioOff, " and when it's on is ", ratioGreen)

	savePerfMetrics(ratioGreen, ratioOff, s)

	// What we are looking for is that nothing is reflecting back, even in a small area (if only something small is in the field of view). Any detected color change is a good case, because that means the screen is working.
	if ratioGreen < ratioOff*1.05 {
		s.Fatalf("Detected NO reflection from panel %s", getPanelName(ctx))
	}
}

func openGreenFullScreen(ctx context.Context, cr *chrome.Chrome) error {
	const html = "<style>body { background-color: green; }</style>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, html)
	}))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL)
	if err != nil {
		return errors.Wrap(err, "Creating renderer failed")
	}
	defer conn.Close()

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.Accel(ctx, "f11"); err != nil {
		return errors.Wrap(err, "failed to type fullscreen hotkey")
	}
	return nil
}

func takeCameraShot(ctx context.Context, cr *chrome.Chrome, imgPath string) error {
	captureCmd := []string{"--user=arc-camera", "cros_camera_test",
		"--gtest_filter=Camera3StillCaptureTest/Camera3DumpSimpleStillCaptureTest.DumpCaptureResult/0",
		"--camera_facing=front",
		"--dump_still_capture_path=" + imgPath,
		"--connect_to_camera_service=false"}
	testing.ContextLog(ctx, "Running ", strings.Join(captureCmd, " "))
	if err := testexec.CommandContext(ctx, "sudo", captureCmd...).Run(); err != nil {
		return errors.Wrap(err, "failed to capture camera shot")
	}
	return nil
}

func getGreenToOtherColorRatio(ctx context.Context, imgPath string, s *testing.State) float64 {
	cmd := testexec.CommandContext(ctx, "convert", imgPath, "-colorspace", "RGB", "-channel", "G", "-separate", "-format", "\"%[mean]\"", "info:")
	out, err := cmd.Output()
	if err != nil {
		return 0.0
	}
	rawValue := strings.Trim(strings.TrimSpace(string(out)), "\"")
	greenness, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return 0.0
	}

	// Get the average of the red and blue channels to compare it against the green channel.
	cmd = testexec.CommandContext(ctx, "convert", imgPath, "-colorspace", "RGB", "-channel", "R", "-separate", "-format", "\"%[mean]\"", "info:")
	out, err = cmd.Output()
	if err != nil {
		return 0
	}
	rawValue = strings.Trim(strings.TrimSpace(string(out)), "\"")
	redness, _ := strconv.ParseFloat(rawValue, 64)

	cmd = testexec.CommandContext(ctx, "convert", imgPath, "-colorspace", "RGB", "-channel", "B", "-separate", "-format", "\"%[mean]\"", "info:")
	out, err = cmd.Output()
	if err != nil {
		return 0
	}
	rawValue = strings.Trim(strings.TrimSpace(string(out)), "\"")
	blueness, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return 0
	}

	averageNonGreen := (redness + blueness) / 2

	ratio := float64(greenness) / float64(averageNonGreen)
	return ratio
}

func getPanelName(ctx context.Context) string {
	connectors, err := graphics.ModetestConnectors(ctx)
	if err != nil {
		return "Failed to get the panel name" + err.Error()
	}

	for _, connector := range connectors {
		if strings.Contains(connector.Name, "eDP") {
			return connector.Edid.ManufacturerName + " " + strconv.Itoa(int(connector.Edid.ModelNumber))
		}
	}

	return "Unknown"
}

func savePerfMetrics(ratioGreen, ratioOff float64, s *testing.State) {
	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed to save perf data: ", err)
		}
	}()

	var ratio float64
	if ratioOff == 0 {
		if ratioGreen == 0 {
			ratio = 0
		} else {
			// 100 is a relatively high ratio to represent inf, which happens when ratioOff is 0, at a very black screen.
			ratio = 100
		}
	} else {
		ratio = ratioGreen / ratioOff
	}

	pv.Set(perf.Metric{
		Name:      "green_to_off_ratio",
		Unit:      "ratio",
		Direction: perf.BiggerIsBetter,
	}, ratio)

	pv.Set(perf.Metric{
		Name:      "ratioGreen",
		Unit:      "ratio",
		Direction: perf.BiggerIsBetter,
	}, ratioGreen)

	pv.Set(perf.Metric{
		Name:      "ratioOff",
		Unit:      "ratio",
		Direction: perf.BiggerIsBetter,
	}, ratioOff)
}
