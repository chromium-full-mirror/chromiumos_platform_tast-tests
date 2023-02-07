// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

var (
	chameleonHost = testing.RegisterVarString(
		"graphics.chameleon_host",
		"",
		"Hostname for Chameleon (optional/not currently used)")

	chameleonIP = testing.RegisterVarString(
		"graphics.chameleon_ip",
		"",
		"Local IP address of Chameleon (required)")

	chameleonPort = testing.RegisterVarString(
		"graphics.chameleon_port",
		"9992",
		"Port for chameleond on Chameleon (optional/used)")

	stableVideoTimeout      = 30.0
	chameleonLogsCursorName = "TAST"
)

// DisplayMode is an enum used to describe Chrome's display modes.
type DisplayMode int

const (
	// Undefined is the default mode
	Undefined DisplayMode = iota
	// UIExtended sets the UI extended mode
	UIExtended
	// UIMirror sets the UI to mirror mode
	UIMirror
	// AlternateExtendedMirror is used to alterate between modes
	AlternateExtendedMirror
)

// ChameleonTest is used to describe the config used to run each test.
type ChameleonTest struct {
	Port       string      // The port name
	Iterations int         // Number of iterations for stress tests
	Display    DisplayMode // The initial display mode
}

// RGB is an in-memory image whose At method returns color.RGBA values.
// This is used for the Chameleon which return RGB pixels for its screenshots.
// We need to implement the ColorModel(), Bounds(), and At() functions to be
// able to encode and save PNG images.
type RGB struct {
	// Pix holds the image's pixels, in R, G, B order.
	Pix []uint8
	// Rect is the image's bounds.
	Rect image.Rectangle
	image.Image
}

// ColorModel returns the color model RGBA for the image
func (p *RGB) ColorModel() color.Model { return color.RGBAModel }

// Bounds gets the dimensions of the image
func (p *RGB) Bounds() image.Rectangle { return p.Rect }

// At gets the color at an (x, y) of the image
func (p *RGB) At(x, y int) color.Color {
	if !(image.Point{x, y}.In(p.Rect)) {
		return color.RGBA{}
	}
	i := 3 * ((x - p.Rect.Min.X) + (y-p.Rect.Min.Y)*p.Rect.Dx())
	return color.RGBA{p.Pix[i], p.Pix[i+1], p.Pix[i+2], 0xff}
}

// ChameleonGetScreenshot gets what chameleon sees
func ChameleonGetScreenshot(ctx context.Context, cham chameleon.Chameleond, port chameleon.PortID, chamPath string) error {
	pixels, err := cham.DumpPixels(ctx, port)
	if err != nil {
		return errors.Errorf("chameleon could not take screenshot: %s", err)
	}

	width, height, err := cham.DetectResolution(ctx, port)
	if err != nil {
		return errors.Errorf("failed to detect resolution: %s", err)
	}
	imgDim := image.Rect(0, 0, width, height)
	img := RGB{Pix: pixels, Rect: imgDim}

	outImage, err := os.Create(chamPath)
	if err != nil {
		return errors.Errorf("failed to create path %s: %s", chamPath, err)
	}
	defer outImage.Close()

	err = png.Encode(outImage, &img)
	if err != nil {
		return errors.Errorf("image did not get encoded: %s", err)
	}
	return nil
}

// SwitchDisplayMode changes the display mode
// mirrorMode set to true sets it to mirror mode, false sets it to extended mode
func SwitchDisplayMode(ctx context.Context, cr *chrome.Chrome, setMirrorMode bool) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Errorf("failed to get test API connection: %s", err)
	}

	ui := uiauto.New(tconn)
	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch os-settings Settings page")
	}

	deviceFinder := nodewith.Name("Device").Role(role.Link)
	err = ui.LeftClick(deviceFinder)(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to launch device page")
	}

	displayFinder := nodewith.Name("Displays").Role(role.Link).Ancestor(ossettings.WindowFinder)
	err = ui.LeftClick(displayFinder)(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to launch display page")
	}

	displayParams := nodewith.Name("Mirror Built-in display").Role(role.CheckBox).Ancestor(ossettings.WindowFinder)
	nodeInfo, err := ui.Info(ctx, displayParams)

	if (setMirrorMode && nodeInfo.Checked == "false") || (!setMirrorMode && nodeInfo.Checked == "true") {
		if err = ui.LeftClick(displayParams)(ctx); err != nil {
			return errors.Wrap(err, "failed to click mirror display")
		}
	}
	if err = settings.Close(ctx); err != nil {
		return err
	}

	// We expect the display has changed modes, we poll for the . Return err after poll timeout.
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		internalDisplayInfo, err := display.GetInternalInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get display infos in mirror mode")
		}

		if setMirrorMode && internalDisplayInfo.ID != internalDisplayInfo.MirroringSourceID {
			return errors.Errorf("Set to mirror mode, but unexpected mirror source ID: got %s, want %s", internalDisplayInfo.MirroringSourceID, internalDisplayInfo.ID)
		}

		if !setMirrorMode && internalDisplayInfo.ID == internalDisplayInfo.MirroringSourceID {
			return errors.New("Set to extended mode, but mirror source ID isn't an empty string")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}

// ChameleonPlug plugs in chameleon and ensures stable video input.
func ChameleonPlug(ctx context.Context, cham chameleon.Chameleond, port chameleon.PortID) error {
	err := cham.Plug(ctx, port)
	if err != nil {
		return errors.Errorf("failed to plug in a physically plugged port %d: %s", port, err)
	}

	isVideoStable, err := cham.WaitVideoInputStable(ctx, port, stableVideoTimeout)
	if err != nil {
		cham.Unplug(ctx, port)
		return errors.Errorf("failed to wait for stable video input: %s", err)
	}
	if !isVideoStable {
		cham.Unplug(ctx, port)
		return errors.Errorf("video not stable after %.2f seconds", stableVideoTimeout)
	}

	return nil
}

// ChameleonGetURL retrieves the Chameleon's IP and port to create a URL.
func ChameleonGetURL() (string, error) {
	if net.ParseIP(chameleonIP.Value()) == nil {
		return "", errors.Errorf("failed to get chameleon ip. The Chameleon ip: %s", chameleonIP.Value())
	}
	chamIP := chameleonIP.Value()
	chamPort := chameleonPort.Value()
	chamURL := net.JoinHostPort(chamIP, chamPort)
	return chamURL, nil
}

// ChameleonGetConnection retrieves the connected Chameleond instance and resets the logs.
func ChameleonGetConnection(ctx context.Context) (chameleon.Chameleond, error) {
	chamURL, err := ChameleonGetURL()
	if err != nil {
		return nil, errors.Errorf("failed to get the Chameleon's URL: %s", err)
	}
	cham, err := chameleon.NewChameleond(ctx, chamURL)
	if err != nil {
		return nil, errors.Errorf("failed to connect to Chameleon: %s", err)
	}

	// Reset logs so we can read it later just for this test.
	cham.GetChameleondLogs(ctx, chameleonLogsCursorName)

	if err = cham.Reset(ctx); err != nil {
		return nil, errors.Errorf("failed to reset Chameleon: %s", err)
	}

	testing.ContextLog(ctx, "Connected to Chameleon")
	return cham, nil
}

// ChameleonShouldUsePort determines whether a port is physically plugged for usage.
func ChameleonShouldUsePort(ctx context.Context, cham chameleon.Chameleond, portStr string) (bool, chameleon.PortID, error) {
	var port chameleon.PortID
	splitIdx := strings.IndexAny(portStr, "0123456789")
	portType := portStr[:splitIdx]
	portIdx, err := strconv.Atoi(portStr[splitIdx:])
	if err != nil {
		return false, port, err
	}
	portIdx--

	portMap, err := cham.FetchSupportedPortIDsByType(ctx)
	if err != nil {
		return false, port, errors.Errorf("failed to fetch supported ports by type: %s", err)
	}

	if portType == "dp" && len(portMap[chameleon.PortTypeDP]) > portIdx {
		port = portMap[chameleon.PortTypeDP][portIdx]
	} else if portType == "hdmi" && len(portMap[chameleon.PortTypeHDMI]) > portIdx {
		port = portMap[chameleon.PortTypeHDMI][portIdx]
	} else {
		return false, port, errors.Errorf("there is no support for DP or HDMI: %s", err)
	}

	err = cham.Plug(ctx, port)
	if err != nil {
		return false, port, errors.Errorf("failed to plug the port %d : %s", port, err)
	}

	isVideoStable, err := cham.WaitVideoInputStable(ctx, port, stableVideoTimeout)
	cham.Unplug(ctx, port)

	return isVideoStable, port, err
}

// ChameleonResizePng resizes a png to the proper format for ChameleonPerceptualDiff
// TODO(b:263161774): update perceptualdiff and use the --scale option instead of resizing.
func ChameleonResizePng(ctx context.Context, src, dst string, width, height int) error {
	imageSize := fmt.Sprintf("%dx%d!", width, height)
	cmd := testexec.CommandContext(ctx, "convert", "-channel", "RGB", "-colorspace", "RGB", "-depth", "8", "-resize", imageSize, src, dst)
	return cmd.Run(testexec.DumpLogOnError)
}

// ChameleonPerceptualDiff compares two images using perceptually based image metric
func ChameleonPerceptualDiff(ctx context.Context, imagePath1, imagePath2, outDir string, threshold int) (bool, error) {
	pixelThreshold := strconv.Itoa(threshold)

	resizedPath1 := filepath.Join(outDir, "resized1.png")
	resizedPath2 := filepath.Join(outDir, "resized2.png")

	err := ChameleonResizePng(ctx, imagePath1, resizedPath1, 100, 100)
	if err != nil {
		return false, errors.Errorf("image at %s failed to resize: %s", imagePath1, err)
	}
	err = ChameleonResizePng(ctx, imagePath2, resizedPath2, 100, 100)
	if err != nil {
		return false, errors.Errorf("image at %s failed to resize: %s", imagePath2, err)
	}

	cmd := testexec.CommandContext(ctx, "perceptualdiff", "-threshold", pixelThreshold, resizedPath1, resizedPath2)
	err = cmd.Run()
	if err != nil {
		return false, nil
	}
	return true, nil
}

// ChameleonSaveLogsAndOutputOnError get the logs from Chameleon and saves to chameleon_logs.txt file. If there is an error, it prints the logs to stdout as well.
func ChameleonSaveLogsAndOutputOnError(ctx context.Context, cham chameleon.Chameleond, isError bool, outDir string) {
	logs, err := cham.GetChameleondLogs(ctx, chameleonLogsCursorName)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get and print Chameleon logs: ", err)
		logs = "Failed to get and print Chameleon logs" + err.Error()
	}

	if isError {
		testing.ContextLog(ctx, "Chameleon logs: ", logs)
	}
	if err := ioutil.WriteFile(filepath.Join(outDir, "chameleon_logs.txt"),
		[]byte(logs), 0644); err != nil {
		testing.ContextLog(ctx, "Failed to write Chameleon logs: ", err)
	}

}
