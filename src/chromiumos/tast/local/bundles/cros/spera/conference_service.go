// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/common/bond"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/spera/conference"
	"chromiumos/tast/local/bundles/cros/spera/conference/zoomserver"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/typecutils"
	"chromiumos/tast/local/ui/cujrecorder"
	pb "chromiumos/tast/services/cros/spera"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterConferenceService2Server(srv, &ConferenceService{s: s})
		},
		Vars: []string{
			// mode is optional. Expecting "tablet" or "clamshell".
			"spera.cuj_mode",
			// Optional. Expecting "enable" or "disable", default is "disable".
			"spera.collectTrace",
			// Optional. Expecting "true" or "false", default is "false".
			"spera.collectWebRTCInternals",
			// CrOS login credentials.
			"ui.cujAccountPool",
			// Credentials used to join Google Meet. It might be different with CrOS login credentials.
			"spera.meet_account",
			"spera.meet_password",
			// Credentials for BOND API.
			"spera.GoogleMeetCUJ.bond_enabled",
			"spera.GoogleMeetCUJ.bond_key",

			// Static Google meet rooms with different participant number have been created.
			// They have different URLs. spera.meet_url can be used to run a specific subtest but
			// assigning urls to different vars will be easier when running with spera.GoogleMeetCUJ.*.
			// Each of the folliwng vars can be assigned with mutiple URLs, seperated by comma.
			// Test can retry another url if one fails.
			// - Primary URLs: use these URLs first.
			"spera.meet_url",
			"spera.meet_url_two",
			"spera.meet_url_small",
			"spera.meet_url_large",
			"spera.meet_url_class",
			// - Secondary URLs: only used when primary ones fail.
			"spera.meet_url_secondary",
			"spera.meet_url_two_secondary",
			"spera.meet_url_small_secondary",
			"spera.meet_url_large_secondary",
			"spera.meet_url_class_secondary",

			// The total timeout and inteval when trying different URLs if one fails.
			"spera.meet_url_retry_timeout",
			"spera.meet_url_retry_interval",
			// Zoom meet bot server address.
			"spera.zoom_bot_server",
			"spera.zoom_bot_token",

			// Optional. Expecting "google", "external", default is "google".
			"spera.Conference.web_source",
		},
	})
}

type ConferenceService struct {
	s *testing.ServiceState
}

func confereceChromeOpts(accountPool, cameraVideoPath string) []chrome.Option {
	chromeArgs := chromeArgsWithFileCameraInput(cameraVideoPath)
	return []chrome.Option{
		// Make sure we are running new chrome UI when tablet mode is enabled by CUJ test.
		// Remove this when new UI becomes default.
		chrome.EnableFeatures("WebUITabStrip"),
		chrome.KeepState(),
		chrome.ARCSupported(),
		chrome.GAIALoginPool(accountPool),
		chrome.ExtraArgs(chromeArgs...)}
}

// chromeArgsWithFileCameraInput returns Chrome extra args as string slice
// for video test with a Y4M/MJPEG fileName streamed as live camera input.
func chromeArgsWithFileCameraInput(fileName string) []string {
	if fileName == "" {
		return []string{}
	}
	return []string{
		// See https://webrtc.github.io/webrtc-org/testing/.
		// Feed a test pattern to getUserMedia() instead of live camera input.
		"--use-fake-device-for-media-stream",
		// Feed a Y4M/MJPEG test file to getUserMedia() instead of live camera input.
		"--use-file-for-fake-video-capture=" + fileName,
	}
}

// newConferenceChrome returns a new Chrome instance with custom options for confernce cuj,
// including setting whether to use fake camera and lacros browser.
func newConferenceChrome(ctx context.Context, accountPool, cameraVideoPath string, bt browser.Type) (cr *chrome.Chrome, err error) {
	opts := confereceChromeOpts(accountPool, cameraVideoPath)
	lacrosCfg := lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.LacrosEnableFeatures("WebUITabStrip")))
	cr, err = browserfixt.NewChrome(ctx, bt, lacrosCfg, opts...)
	if err != nil {
		return cr, errors.Wrap(err, "failed to restart Chrome")
	}
	preTest(ctx)
	return cr, nil
}

func preTest(ctx context.Context) {
	// ChargeBatteryCapacityBeforePowerTest is common code for all spera tests.
	// If there is a new spera case, need to confirm that ChargeBatteryCapacityBeforePowerTest
	// is added to the pretest function.
	if err := cuj.ChargeBatteryCapacityBeforePowerTest(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to charge battery capacity before power test: ", err)
	}

	// Wait for cpu to idle before test.
	if err := cpu.WaitUntilIdle(ctx); err != nil {
		// Log the cpu idle wait failure instead of make it fatal.
		testing.ContextLog(ctx, "Failed to wait for CPU to become idle: ", err)
	}
}

const tmpDir = "/tmp"

// The default web source is the google websites.
var webSource = cuj.GoogleWebSource

func (s *ConferenceService) RunGoogleMeetScenario(ctx context.Context, req *pb.MeetScenarioRequest) (*empty.Empty, error) {
	roomType := conference.RoomType(req.RoomType)
	isNoRoom := roomType == conference.NoRoom
	meet, err := conference.GetGoogleMeetConfig(ctx, s.s, roomType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get meet config")
	}
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get outdir from context")
	}
	traceConfigPath := ""
	if collect, ok := s.s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = tmpDir + "/" + cujrecorder.SystemTraceConfigFile
	}
	v, ok := s.s.Var("spera.Conference.web_source")
	if ok && strings.ToLower(v) == string(cuj.ExternalWebSource) {
		webSource = cuj.ExternalWebSource
	}
	run := func(ctx context.Context, roomURL string) error {
		accountPool, ok := s.s.Var("ui.cujAccountPool")
		if !ok {
			return errors.New("failed to get variable ui.cujAccountPool")
		}
		bt := browser.TypeAsh
		if req.IsLacros {
			bt = browser.TypeLacros
		}
		cr, err := newConferenceChrome(ctx, accountPool, req.CameraVideoPath, bt)
		if err != nil {
			return errors.Wrap(err, "failed to new Chrome")
		}
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to test API")
		}
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to initialize keyboard input")
		}
		defer kb.Close()
		cleanupCtx := ctx
		ctx, cancelTablet := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancelTablet()

		tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.s.Var, "spera.cuj_mode")
		if err != nil {
			return errors.Wrap(err, "failed to enable tablet mode")
		}
		defer resetTabletMode(cleanupCtx)

		var uiHandler cuj.UIActionHandler
		if tabletMode {
			cleanup, err := display.RotateToLandscape(ctx, tconn)
			if err != nil {
				return errors.Wrap(err, "failed to rotate display to landscape")
			}
			defer cleanup(cleanupCtx)
			if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to create tablet action handler")
			}
		} else {
			if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to create clamshell action handler")
			}
		}

		if req.ExtendedDisplay {
			// Unset mirrored display so two displays can show different information.
			if err := typecutils.SetMirrorDisplay(ctx, tconn, false); err != nil {
				return errors.Wrap(err, "failed to unset mirror display")
			}
			expectedDisplayMode := display.DisplayMode{Height: 1080, RefreshRate: 60}
			if err := display.CheckExtendedDisplay(tconn, expectedDisplayMode)(ctx); err != nil {
				return errors.Wrap(err, "failed to check extended display")
			}
		}

		prepare := func(ctx context.Context) (string, conference.Cleanup, error) {
			cleanup := func(ctx context.Context) (err error) {
				// Nothing to clean up at the end of Google Meet conference.
				return nil
			}
			if !isNoRoom && roomURL == "" {
				return "", nil, errors.New("the conference invite link is empty")
			}
			return roomURL, cleanup, nil
		}

		// Creates a Google Meet conference instance which implements conference.Conference methods
		// which provides conference operations.
		gmcli := conference.NewGoogleMeetConference(cr, tconn, kb, uiHandler, bt, roomType, meet, outDir, tabletMode, req.ExtendedDisplay)
		defer gmcli.End(cleanupCtx)
		// Shorten context a bit to allow for cleanup if Run fails.
		ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
		defer cancel()
		testParams := &conference.TestParams{
			Cr:                     cr,
			Conf:                   gmcli,
			Prepare:                prepare,
			Tier:                   cuj.Tier(req.Tier),
			Bt:                     bt,
			RoomType:               roomType,
			OutDir:                 outDir,
			TraceConfigPath:        traceConfigPath,
			TabletMode:             tabletMode,
			CollectWebRTCInternals: meet.CollectWebRTCInternals,
			WebSource:              webSource,
		}
		if err := conference.Run(ctx, testParams); err != nil {
			return errors.Wrap(err, "failed to run Google Meet conference")
		}
		return nil
	}
	if isNoRoom {
		// Without Google Meet, there is no need to assign a meet url.
		if err := run(ctx, ""); err != nil {
			testing.ContextLogf(ctx, "Failed to run conference: %+v", err)
			return nil, err
		}
		return &empty.Empty{}, nil
	}

	runWithMeetUrls := func(ctx context.Context) error {
		if meet.BondEnabled {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
			defer cancel()
			meetlink, cleanupBond, err := generateMeetLinkViaBond(ctx, meet, roomType)
			if err != nil {
				return &conference.BondError{Err: errors.Wrap(err, "failed to create meet link via BOND API")}
			}
			defer cleanupBond(cleanupCtx)
			meet.URLs = []string{meetlink}
		}
		var err error
		for _, url := range meet.URLs {
			testing.ContextLog(ctx, "URL to be tested in the meet url list: ", url)
			err = run(ctx, url)
			if err == nil {
				return nil
			}
			if !conference.IsParticipantError(err) {
				return err
			}
		}
		return err
	}
	// If meet.RetryTimeout equal to 0, don't do any retry.
	if meet.RetryTimeout == 0 {
		testing.ContextLog(ctx, "Start running meet scenario")
		if err := runWithMeetUrls(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to run conference: %+v", err) // Print error with stack trace.
			return nil, err
		}
		return &empty.Empty{}, nil
	}

	var lastError error
	startTime := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := runWithMeetUrls(ctx); err != nil {
			elapsedTime := time.Now().Sub(startTime)
			if elapsedTime < meet.RetryTimeout {
				// Record the complete run result if the failure is not because of timeout.
				lastError = err
			}
			if conference.IsParticipantError(err) || conference.IsBondError(err) {
				testing.ContextLogf(ctx, "Wait %v and try to run meet scenario again; caused by error: %v", meet.RetryInterval, err)
				return err
			}
			return testing.PollBreak(err) // Break if error is not participant number related.
		}
		return nil
	}, &testing.PollOptions{Timeout: meet.RetryTimeout, Interval: meet.RetryInterval}); err != nil {
		// Return test failure reason of last complete run.
		if lastError != nil {
			err = lastError
		}
		testing.ContextLogf(ctx, "Failed to run conference: %+v", err) // Print error with stack trace.
		return nil, err
	}
	return &empty.Empty{}, nil
}

func (s *ConferenceService) RunZoomScenario(ctx context.Context, req *pb.MeetScenarioRequest) (*empty.Empty, error) {
	roomType := conference.RoomType(req.RoomType)

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get outdir from context")
	}
	accountPool, ok := s.s.Var("ui.cujAccountPool")
	if !ok {
		return nil, errors.New("failed to get variable ui.cujAccountPool")
	}
	host, ok := s.s.Var("spera.zoom_bot_server")
	if !ok {
		return nil, errors.New("failed to get variable spera.zoom_bot_server")
	}

	sessionToken, ok := s.s.Var("spera.zoom_bot_token")
	if !ok {
		return nil, errors.New("failed to get variable spera.zoom_bot_token")
	}
	traceConfigPath := ""
	if collect, ok := s.s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = tmpDir + "/" + cujrecorder.SystemTraceConfigFile
	}

	v, ok := s.s.Var("spera.Conference.web_source")
	if ok && strings.ToLower(v) == string(cuj.ExternalWebSource) {
		webSource = cuj.ExternalWebSource
	}

	testing.ContextLog(ctx, "Start zoom meet scenario")
	bt := browser.TypeAsh
	if req.IsLacros {
		bt = browser.TypeLacros
	}
	cr, err := newConferenceChrome(ctx, accountPool, req.CameraVideoPath, bt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to new Chrome")
	}
	account := cr.Creds().User

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API")
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize keyboard input")
	}
	defer kb.Close()
	cleanupCtx := ctx
	ctx, cancelTablet := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancelTablet()

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.s.Var, "spera.cuj_mode")
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable tablet mode")
	}
	defer resetTabletMode(cleanupCtx)

	var uiHandler cuj.UIActionHandler
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			return nil, errors.Wrap(err, "failed to rotate display to landscape")
		}
		defer cleanup(cleanupCtx)
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to create tablet action handler")
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to create clamshell action handler")
		}
	}
	zmcli := conference.NewZoomConference(cr, tconn, kb, uiHandler, tabletMode, roomType, account, outDir)
	defer zmcli.End(cleanupCtx)

	roomSize := conference.ZoomRoomParticipants[roomType] - 1
	prepare := func(ctx context.Context) (string, conference.Cleanup, error) {
		return zoomserver.CreateConference(ctx, roomSize, sessionToken, host)
	}
	// Shorten context a bit to allow for cleanup if Run fails.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()
	testParams := &conference.TestParams{
		Cr:              cr,
		Conf:            zmcli,
		Prepare:         prepare,
		Tier:            cuj.Tier(req.Tier),
		Bt:              bt,
		RoomType:        roomType,
		OutDir:          outDir,
		TraceConfigPath: traceConfigPath,
		TabletMode:      tabletMode,
		WebSource:       webSource,
	}
	if err := conference.Run(ctx, testParams); err != nil {
		return nil, errors.Wrap(err, "failed to run Zoom conference")
	}

	return &empty.Empty{}, nil
}

func generateMeetLinkViaBond(ctx context.Context, meet conference.GoogleMeetConfig, roomType conference.RoomType) (meetLink string, cleanup func(ctx context.Context), err error) {
	var (
		bondConn        *bond.Client
		bondMeetingCode string
		numFailures     int
	)
	cleanupfunc := func(ctx context.Context) {
		if bondConn != nil {
			if bondMeetingCode != "" {
				bondConn.RemoveAllBotsFromConference(ctx, bondMeetingCode)
			}
			bondConn.Close()
		}
	}
	// Connect.
	bondConn, err = bond.NewClient(ctx, bond.WithCredsJSON(meet.BondCreds), bond.WithExternalEndpoint())
	if err != nil {
		return "", cleanupfunc, errors.Wrap(err, "BOND API2: Failed to connect")
	}
	defer func(ctx context.Context) {
		if err != nil {
			bondConn.Close()
		}
	}(ctx)

	// Create room with bots.
	botsDuration := 60 * time.Minute // one hour long by default.
	deadline, ok := ctx.Deadline()
	if ok {
		botsDuration = deadline.Add(90 * time.Second).Sub(time.Now())
	}
	numBots := conference.GoogleMeetRoomParticipants[roomType] - 1 // one of participants is the test itself
	bondMeetingCode, numFailures, err = bondConn.CreateConferenceWithBots(ctx, numBots, botsDuration)
	defer func(ctx context.Context) {
		if err != nil {
			bondConn.RemoveAllBotsFromConference(ctx, bondMeetingCode)
		}
	}(ctx)

	if err != nil || numFailures > 0 {
		return "", cleanupfunc, errors.Wrapf(err, "BOND API2: %d bots failed to connect", numFailures)
	}
	testing.ContextLogf(ctx, "BOND API2: Created conference: %+v and added %d bots for the duration of %v", bondMeetingCode, numBots, botsDuration)

	// Make the room created by BOND the first one to try.
	meetLink = fmt.Sprintf("https://meet.google.com/%s", bondMeetingCode)

	return meetLink, cleanupfunc, nil
}
