// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeAECDump,
		Desc:         "Verifies AEC dump works",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546",
		Fixture:      fixture.Chrome(),
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Data:         []string{data.AudioLong16Wav},
		Timeout:      1 * time.Minute,
	})
}

var (
	createDiagnosticAudioRecordingsSection  = nodewith.Name("Create diagnostic audio recordings").Role(role.DisclosureTriangle)
	enableDiagnosticAudioRecordingsCheckbox = nodewith.Name("Enable diagnostic audio recordings").Role(role.CheckBox)
)

func ChromeAECDump(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("cr.TestAPIConn: ", err)
	}
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.User())
	if err != nil {
		s.Fatal("Cannot get Downloads path: ", err)
	}

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	webrtcInternals, err := cr.Browser().NewConn(ctx, "chrome://webrtc-internals")
	if err != nil {
		s.Fatal("Cannot open webrtc-internals: ", err)
	}
	defer webrtcInternals.Close()
	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	if err := uiauto.NamedCombine("Enable diagnostic audio recordings",
		ui.WaitUntilExists(createDiagnosticAudioRecordingsSection),
		ui.DoDefault(createDiagnosticAudioRecordingsSection),
		ui.WaitUntilExists(enableDiagnosticAudioRecordingsCheckbox),
		ui.DoDefault(enableDiagnosticAudioRecordingsCheckbox),
	)(ctx); err != nil {
		s.Fatal("Cannot start WebRTC audio dump: ", err)
	}

	// Find the files app dialog.
	saver, err := filesapp.App(ctx, tconn, filesapp.FileSaverPseudoAppID)
	if err != nil {
		s.Fatal(`Cannot locate "Save file as" dialog: `, err)
	}
	saver = saver.WithTimeout(10 * time.Second)
	saveButton := nodewith.Role(role.Button).Name("Save")
	if err := uiauto.NamedCombine("Save audio_debug",
		saver.OpenDir("Downloads", "Downloads"),
		saver.WaitUntilExists(saveButton),
		saver.LeftClick(saveButton),
	)(ctx); err != nil {
		s.Fatal("Cannot select diagnostic audio recordings filename: ", err)
	}

	// Open an arbitrary web page that has access to navigator.mediaDevices.
	recorderPage, err := cr.Browser().NewTab(ctx, "chrome://version")
	if err != nil {
		s.Fatal("Cannot open recorder: ", err)
	}
	defer recorderPage.Close()
	if err := recorderPage.Eval(ctx,
		`var stream;
	async function f() {
		stream = await navigator.mediaDevices.getUserMedia({audio: true});
	}
	f()`,
		nil); err != nil {
		s.Fatal("Cannot start capture in Chrome: ", err)
	}

	playerPage, err := cr.Browser().NewTab(ctx, server.URL+"/"+data.AudioLong16Wav)
	if err != nil {
		s.Fatal("Cannot open player: ", err)
	}
	defer playerPage.Close()

	const playbackDuration = 20 * time.Second
	s.Log("Play audio for ", playbackDuration)
	// GoBigSleepLint: Play audio.
	testing.Sleep(ctx, playbackDuration)

	if err := recorderPage.CloseTarget(ctx); err != nil {
		s.Fatal("Cannot close recorder page: ", err)
	}
	if err := playerPage.CloseTarget(ctx); err != nil {
		s.Fatal("Cannot close player page: ", err)
	}
	if err := webrtcInternals.CloseTarget(ctx); err != nil {
		s.Fatal("Cannot close webrtc-internals: ", err)
	}

	if err := fsutil.CopyDir(downloadsPath, filepath.Join(s.OutDir(), "webrtc-internals-dump")); err != nil {
		s.Fatal("Cannot copy dump: ", err)
	}
}
