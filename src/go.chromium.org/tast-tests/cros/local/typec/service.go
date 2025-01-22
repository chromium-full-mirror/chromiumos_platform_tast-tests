// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			typec.RegisterServiceServer(srv, &Service{s: s})
		},
	})
}

// Service implements tast.cros.typec.Service.
type Service struct {
	s    *testing.ServiceState
	cr   *chrome.Chrome
	conn *chrome.Conn
}

// NewChromeLoginWithPeripheralDataAccess logs in to Chrome as a fake user, but before that, enables the DevicePciPeripheralDataAccess setting.
func (c *Service) NewChromeLoginWithPeripheralDataAccess(ctx context.Context, req *typec.KeyPath) (_ *empty.Empty, errRet error) {
	// Get to the Chrome login screen.
	cr, err := chrome.New(ctx, chrome.DeferLogin(), chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome at login screen")
	}
	defer func() {
		if errRet != nil {
			cr.Close(ctx)
		}
	}()

	// Enable the setting.
	if err := typecutils.EnablePeripheralDataAccess(ctx, req.Path); err != nil {
		return nil, errors.Wrap(err, "failed to enable peripheral data access setting")
	}

	if err := cr.ContinueLogin(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to login")
	}

	c.cr = cr

	return &empty.Empty{}, nil
}

// SetMirrorModeDisplay sets dut to mirror mode.
func (c *Service) SetMirrorModeDisplay(ctx context.Context, req *typec.KeyPath) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	if err := typecutils.SetMirrorDisplay(ctx, tconn, req.SetMode); err != nil {
		return nil, errors.Wrap(err, "failed to set mirror mode")
	}
	return &empty.Empty{}, nil
}

// PlayLocalVideo plays local video and verifies whether local video is playing or not.
func (c *Service) PlayLocalVideo(ctx context.Context, req *typec.KeyPath) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, c.cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user's Download path")
	}

	srv := httptest.NewServer(http.FileServer(http.Dir(downloadsPath)))
	defer srv.Close()
	url := srv.URL + "/video.html"
	conn, err := c.cr.NewConn(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load video.html")
	}
	c.conn = conn
	if err := conn.Call(ctx, nil, "playRepeatedly", req.Path, true, true); err != nil {
		return nil, errors.Wrap(err, "failed to play video")
	}

	playStatus := true
	videoElement := "document.getElementsByTagName('video')[0]"
	if err := conn.Eval(ctx, videoElement+".paused", &playStatus); err != nil {
		return nil, errors.Wrap(err, "failed to get video play status")
	}
	if playStatus {
		return nil, errors.New("failed: video is not played")
	}
	return &empty.Empty{}, nil
}

// DownloadsPath returns downloads path.
func (c *Service) DownloadsPath(ctx context.Context, req *empty.Empty) (*typec.TypecResponse, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, c.cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve users Downloads path")
	}
	return &typec.TypecResponse{DownloadsPath: downloadsPath}, nil
}

// VerifyMirrorMode veifies whether dut is set to mirror mode or not.
func (c *Service) VerifyMirrorMode(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	if err := typecutils.VerifyMirrorMode(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to verify mirror mode")
	}
	return &empty.Empty{}, nil
}

// PlayVideo resumes the paused video.
func (c *Service) PlayVideo(ctx context.Context, req *typec.KeyPath) (*empty.Empty, error) {
	var playStatus bool
	videoElement := "document.getElementsByTagName('video')[0]"
	if err := c.conn.Eval(ctx, videoElement+".paused", &playStatus); err != nil {
		return nil, errors.Wrap(err, "failed to get video play status")
	}
	if playStatus {
		videoElement := "document.getElementsByTagName('video')[0].play()"
		if err := c.conn.Eval(ctx, videoElement, nil); err != nil {
			return nil, errors.Wrap(err, "failed to play video")
		}
	}
	return &empty.Empty{}, nil
}

// VerifyWindowOnDisplay verifies whether the active window is present on display or not.
func (c *Service) VerifyWindowOnDisplay(ctx context.Context, req *typec.KeyPath) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	win, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get active window")
	}
	info, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get extrenal display info")
	}

	if err := typecutils.VerifyWindowOnDisplay(ctx, tconn, win.ARCPackageName, info[req.DisplayInfo].ID); err != nil {
		return nil, errors.Wrap(err, "failed to verify window on display")
	}
	return &empty.Empty{}, nil
}

// VerifyEdpPrimary verifies whether dut is in primary mode or not
func (c *Service) VerifyEdpPrimary(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	if err := typecutils.VerifyEdpPrimary(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to Verify extended mode display")
	}
	return &empty.Empty{}, nil
}

// SetActiveNodeByUI selectes the required audio output node using UI.
func (c *Service) SetActiveNodeByUI(ctx context.Context, req *typec.KeyPath) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}
	quicksettings.Hide(ctx, tconn)
	quicksettings.Show(ctx, tconn)

	if err := quicksettings.SelectAudioOption(ctx, tconn, req.AudioNode); err != nil {
		return nil, errors.Wrapf(err, "failed to select %q as audio output node", req.AudioNode)
	}
	quicksettings.Hide(ctx, tconn)
	return &empty.Empty{}, nil
}

// DisplayName will return display name of connected external display.
func (c *Service) DisplayName(ctx context.Context, req *typec.KeyPath) (*typec.TypecResponse, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create test API connection")
	}

	displayInfo, err := display.GetInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get external display name")
	}
	if len(displayInfo) < 2 {
		return nil, errors.New("failed to find external display")
	}
	displayName := displayInfo[req.DisplayInfo].Name

	return &typec.TypecResponse{DisplayName: displayName}, nil
}

// AudioCrasSelectedOutputDevice will return selected audio device name
// and audio device type.
func (c *Service) AudioCrasSelectedOutputDevice(ctx context.Context, req *empty.Empty) (*typec.TypecResponse, error) {
	// Get Current active node.
	cras, err := audio.NewCras(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Cras object")
	}
	outDeviceName, outDeviceType, err := cras.SelectedOutputDevice(ctx)
	testing.ContextLog(ctx, "outDeviceType:", outDeviceType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the selected audio device")
	}
	return &typec.TypecResponse{DeviceName: outDeviceName, DeviceType: outDeviceType}, nil
}

// VerifyFirstRunningDevice will check for audio routing device status.
func (c *Service) VerifyFirstRunningDevice(ctx context.Context, req *typec.KeyPath) (*empty.Empty, error) {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
		if err != nil {
			return errors.Wrap(err, "failed to detect running output device")
		}

		if deviceName := req.AudioNode; deviceName != devName {
			return errors.Wrapf(err, "failed to route the audio through expected audio node: got %q; want %q", devName, deviceName)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to check audio running device")
	}
	return &empty.Empty{}, nil
}

// NewChromeLogin is default chrome login.
func (c *Service) NewChromeLogin(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	// Get to the Chrome login screen with proxy.
	cr, err := chrome.New(ctx, chrome.ExtraArgs("--autoplay-policy=no-user-gesture-required"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome at login screen")
	}
	c.cr = cr
	return &empty.Empty{}, nil
}

// ReconnectChromeConnection reconnects to closed chrome connection.
func (c *Service) ReconnectChromeConnection(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.cr == nil {
		return nil, errors.New("Chrome not available")
	}
	if err := c.cr.Reconnect(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reconnect chrome")
	}
	return &empty.Empty{}, nil
}

// NewConnectionForTarget updates new connection for existing target.
func (c *Service) NewConnectionForTarget(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	conn, err := c.cr.NewConnForTarget(ctx, chrome.MatchAllPages())
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to browser tab")
	}
	c.conn = conn
	return &empty.Empty{}, nil
}
