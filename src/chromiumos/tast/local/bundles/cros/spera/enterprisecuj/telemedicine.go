// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterprisecuj

import (
	"context"

	cx "chromiumos/tast/local/bundles/cros/spera/enterprisecuj/citrix"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"
)

// Since the resolution of each DUT may be different, each coordinate file must be recorded separately.
// Now only recording is made for the hatch device. If replaying on other machines, the case may fail due to wrong coordinates.
// TODO(b/243053294): Since these file may change frequently, it's better the test download the files based on a var.
const (
	telemedicineCoordinatesFile = "citrix/telemedicine_coordinates.json"
	telemedicineUIWaitFile      = "citrix/telemedicine_uiwait.json"
)

// TelemedicineData lists all icon data used in the telemedicine cuj.
var TelemedicineData = []string{
	// The following are icons used by uidetection in the telemedicine cuj.
	cx.IconToolbar,
	cx.IconSearch,
	cx.IconDesktop,
	cx.IconTrackerCancel,
	cx.IconChromeTaskManager,
	cx.IconChromeActive,
	cx.IconZoomJoin,
	// The following are files used in replay mode.
	telemedicineCoordinatesFile,
	telemedicineUIWaitFile,
}

// Test scenario for Telemedicine CUJ:
// 1. Maximize the Citrix app. All remaining operations are inside Citrix.
// 2. In Citrix client, open 3 browser windows x 2 tabs (substitute for Cerner health app).
// 3. Open Zoom website inside Citrix and start a 2p video call.
// 4. Concurrently type notes at 70 wpm in Google keep and switch between browser windows without occluding Zoom window.

// Run runs scenario for telemedicine cuj.
func (t *TelemedicineScenario) Run(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, citrix *cx.Citrix, p *TestParams) error {
	const noteContent = "Sample note"
	// Substitute Google, Google Photos, Wikipedia, WebMD for health provider website.
	firstChromeURLs := []string{cuj.GoogleURL, cuj.GooglePhotosURL}
	secondChromeURLs := []string{cuj.WikipediaURL, cuj.WebMDURL}
	thirdChromeURLs := []string{cuj.GoogleURL, cuj.WikipediaURL}
	windows := [][]string{firstChromeURLs, secondChromeURLs, thirdChromeURLs}
	openWindows := func(ctx context.Context) error {
		for _, chromeURLs := range windows {
			if err := citrix.OpenChromeWithURLs(chromeURLs)(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	switchWindows := func(ctx context.Context) error {
		for range windows {
			if err := citrix.SwitchWindow()(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	if p.TestMode == cx.ReplayMode {
		if err := citrix.LoadRecordFile(telemedicineCoordinatesFile, telemedicineUIWaitFile); err != nil {
			return err
		}
	}
	return uiauto.NamedCombine("run the telemedicine cuj scenario",
		citrix.ConnectRemoteDesktop(p.DesktopName),
		citrix.CloseAllChromeBrowsers(),
		// 1. Maximize the Citrix app.
		citrix.FullscreenDesktop(),
		citrix.FocusOnDesktop(),
		// 2. In Citrix client, open 3 browser windows x 2 tabs (substitute for Cerner health app).
		openWindows,
		// 3. Open Zoom website inside Citrix and start a 2p video call.
		citrix.OpenZoom(t.zoomInviteLink, t.zoomAccount),
		// 4. Concurrently type notes at 70 wpm in Google keep
		citrix.CreateGoogleKeepNote(noteContent),
		citrix.DeleteGoogleKeepNote(noteContent),
		// 4. Switch between browser windows without occluding Zoom window.
		switchWindows,
		citrix.CloseAllChromeBrowsers(),
	)(ctx)
}

// TelemedicineScenario implements the CitrixScenario interface.
type TelemedicineScenario struct {
	zoomInviteLink string
	zoomAccount    string
}

var _ CitrixScenario = (*TelemedicineScenario)(nil)

// NewTelemedicineScenario creates telemedicine instance which implements CitrixScenario interface.
func NewTelemedicineScenario(zoomInviteLink, zoomAccount string) *TelemedicineScenario {
	return &TelemedicineScenario{
		zoomInviteLink: zoomInviteLink,
		zoomAccount:    zoomAccount,
	}
}
