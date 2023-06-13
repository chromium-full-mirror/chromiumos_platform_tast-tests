// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"bufio"
	"context"
	"io"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TouchBasicsCheck,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Touch screen check basic functionality of the browser",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.TouchScreen()),
		Attr:         []string{"group:intel-nda"},
		Timeout:      5 * time.Minute,
		Fixture:      "chromeLoggedIn",
	})
}

func TouchBasicsCheck(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	info, err := display.GetInternalInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to obtain the internal display info: ", err)
	}
	tsw, err := input.Touchscreen(ctx)
	if err != nil {
		s.Fatal("Failed to open touchscreen device: ", err)
	}
	defer tsw.Close(ctx)

	tcc := tsw.NewTouchCoordConverter(info.Bounds.Size())
	stw, err := tsw.NewSingleTouchWriter()
	if err != nil {
		s.Fatal("Failed to get the single touch event writer: ", err)
	}
	defer stw.Close()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	chromeApp, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find the chrome app: ", err)
	}

	if err := apps.Launch(ctx, tconn, chromeApp.ID); err != nil {
		s.Fatalf("Failed to open %s: %s", chromeApp.Name, err)
	}
	defer apps.Close(cleanupCtx, tconn, chromeApp.ID)

	// Chrome app name doesn't exactly match the chrome shelf name so modify it here for simpler code later.
	if chromeApp.Name == apps.Chrome.Name {
		chromeApp.Name = "Google Chrome"
	}

	cmd, stdout, err := deviceScanner(ctx)
	if err != nil {
		s.Fatal("Failed to get touchscreen scanner: ", err)
	}
	defer cmd.Wait()
	defer cmd.Kill()

	scannerTouchscreen := bufio.NewScanner(stdout)

	nodes := []*nodewith.Finder{
		nodewith.Name("New Tab").Role(role.Button),
		nodewith.Name("Minimize").ClassName("FrameCaptionButton").Role(role.Button),
		nodewith.Name(chromeApp.Name).Role(role.Button).ClassName("ash/ShelfAppButton").First(),
		nodewith.Name("Maximize").ClassName("FrameSizeButton").Role(role.Button),
		nodewith.Name("Close").Role(role.Button).First(),
		nodewith.Name(chromeApp.Name).Role(role.Button).ClassName("ash/ShelfAppButton").First(),
		nodewith.Name("Restore").ClassName("FrameSizeButton").Role(role.Button),
	}

	for _, node := range nodes {
		ui := uiauto.New(tconn)
		nodeLoc, err := ui.Location(ctx, node)
		if err != nil {
			s.Fatal("Failed to get location coordinates: ", err)
		}
		x, y := tcc.ConvertLocation(nodeLoc.CenterPoint())
		tapCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		if err := tapAndVerify(tapCtx, stw, scannerTouchscreen, x, y); err != nil {
			s.Fatal("Failed to tap and verify the touch event: ", err)
		}
	}

	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the windows: ", err)
	}
	w, err := ash.FindWindow(ctx, tconn, func(w *ash.Window) bool { return w.ID == ws[0].ID })
	if err != nil {
		s.Fatal("Failed to find the window: ", err)
	}
	bounds := w.BoundsInRoot

	coordinates := map[string]coords.Point{
		"Center":        bounds.CenterPoint(),
		"Bottom Left":   bounds.BottomLeft(),
		"Bottom Right":  bounds.BottomRight(),
		"Top Left":      bounds.TopLeft(),
		"Top Right":     bounds.TopRight(),
		"Bottom Centre": bounds.BottomCenter(),
		"Right Centre":  bounds.RightCenter(),
		"Top Centre":    bounds.TopCenter(),
		"Left Centre":   bounds.LeftCenter(),
	}
	for pos, coordinate := range coordinates {
		s.Logf("Tapping at %s", pos)
		x, y := tcc.ConvertLocation(coordinate)
		tapCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		if err := tapAndVerify(tapCtx, stw, scannerTouchscreen, x, y); err != nil {
			s.Fatal("Failed to tap and verify the touch event: ", err)
		}
	}
}

// tapAndVerify inject the touch event at coordinate(x, y) and verifies using evtest.
func tapAndVerify(ctx context.Context, stw *input.SingleTouchEventWriter, scanner *bufio.Scanner, x, y input.TouchCoord) error {
	regex := `Event.*time.*code\s(\d*)\s\(` + `BTN_TOUCH` + `\)`
	expMatch := regexp.MustCompile(regex)

	text := make(chan string)
	go func() {
		defer close(text)
		for scanner.Scan() {
			text <- scanner.Text()
		}
	}()
	if err := stw.Move(x, y); err != nil {
		return errors.Wrap(err, "failed to inject touch event")
	}
	if err := stw.End(); err != nil {
		return errors.Wrap(err, "failed to finish the touch event")
	}

	for {
		select {
		case <-ctx.Done():
			return errors.New("did not detect touch event within expected time")
		case out := <-text:
			if expMatch.MatchString(out) {
				return nil
			}
		}
	}
}

// deviceScanner returns the evtest scanner for the touch screen device.
func deviceScanner(ctx context.Context) (*testexec.Cmd, io.Reader, error) {
	foundTS, devPath, err := input.FindPhysicalTouchscreen(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to find device path for the touch screen")
	}
	if !foundTS {
		return nil, nil, errors.New("failed to find physical touch screen")
	}
	cmd := testexec.CommandContext(ctx, "evtest", devPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create stdout pipe")
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start scanner")
	}

	return cmd, stdout, nil
}
