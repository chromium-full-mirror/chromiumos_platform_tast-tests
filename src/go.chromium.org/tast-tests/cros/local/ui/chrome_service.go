// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosinfo"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterChromeServiceServer(srv,
				&ChromeService{sharedObject: common.SharedObjectsForServiceSingleton})
		},
		GuaranteeCompatibility: true,
	})
}

// ChromeService implements tast.cros.ui.ChromeService
type ChromeService struct {
	sharedObject *common.SharedObjectsForService
}

// defaultCreds is the default credentials used for fake logins.
var defaultCreds = chrome.Creds{
	User: "testuser@gmail.com",
	Pass: "testpass",
}

// New logs into Chrome with the supplied chrome options.
func (svc *ChromeService) New(ctx context.Context, req *pb.NewRequest) (_ *empty.Empty, errRet error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	opts, err := toOptions(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert to chrome options")
	}

	var lcfg *lacrosfixt.Config
	var bt browser.Type
	// Enable Lacros iff |req.Lacros| is set.
	if req.GetLacros() != nil {
		bt = browser.TypeLacros
		lcfg, err = toLacrosConfig(req)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert to lacros config")
		}
	} else {
		bt = browser.TypeAsh
		opts = append(opts, chrome.DisableFeatures("LacrosOnly"))
	}

	switch req.GetArcMode() {
	case pb.ArcMode_ARC_MODE_UNSPECIFIED:
		// Do nothing.
	case pb.ArcMode_ARC_MODE_DISABLED:
		opts = append(opts, chrome.ARCDisabled())
	case pb.ArcMode_ARC_MODE_ENABLED:
		opts = append(opts, chrome.ARCEnabled())
	case pb.ArcMode_ARC_MODE_SUPPORTED:
		opts = append(opts, chrome.ARCSupported())
	}

	// By default, this will always create a new chrome session even when there is an existing one.
	// This gives full control of the lifecycle to the end users.
	// Users can use TryReuseSessions if they want to potentially reuse the session.
	cr, err := browserfixt.NewChrome(ctx, bt, lcfg, opts...)
	if err != nil {
		testing.ContextLog(ctx, "Failed to start Chrome")
		return nil, err
	}
	defer func() {
		if errRet != nil {
			cr.Close(ctx)
		}
	}()

	// Check that Lacros is enabled only if requested.
	if bt == browser.TypeLacros {
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create test API connection")
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			info, err := lacrosinfo.Snapshot(ctx, tconn)
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to get lacros info"))
			}
			if len(info.LacrosPath) == 0 {
				return errors.Wrap(err, "lacros is not yet enabled (received empty LacrosPath)")
			}
			return nil
		}, &testing.PollOptions{Interval: 2 * time.Second}); err != nil {
			return nil, errors.Wrapf(err, "lacros is not enabled but requested in %v", req)
		}
	}

	// Store the newly created chrome sessions in the shared object so other services can use it.
	svc.sharedObject.Chrome = cr

	return &empty.Empty{}, nil
}

// Close closes all surfaces and Chrome.
// This will likely be called in a defer in remote tests instead of called explicitly.
func (svc *ChromeService) Close(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	if svc.sharedObject.Chrome == nil {
		return nil, errors.New("Chrome not available")
	}

	err := svc.sharedObject.Chrome.Close(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome: ", err)
	}

	svc.sharedObject.Chrome = nil
	return &empty.Empty{}, err
}

// Reconnect reconnects to the current browser session.
//
// This method is called when connection need to be re-established, e.g. after suspend/resume.
// After the session is reconnected, all existing connections associated with chrome.Chrome instance also
// needs to be re-established. For example, chrome.TestAPIConn(), chrome.NewConn().
//
// Note that this method cannot be used to recover Chrome after crashes since the devtools port may change.
func (svc *ChromeService) Reconnect(ctx context.Context, _ *empty.Empty) (*empty.Empty, error) {
	svc.sharedObject.ChromeMutex.Lock()
	defer svc.sharedObject.ChromeMutex.Unlock()

	if svc.sharedObject.Chrome == nil {
		return nil, errors.New("Chrome not available")
	}

	if err := svc.sharedObject.Chrome.Reconnect(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to reconnect to current browser session")
	}

	return &empty.Empty{}, nil
}

func toOptions(req *pb.NewRequest) ([]chrome.Option, error) {
	// TODO(jonfan): Find a creative way to unit test this function
	// The underlying object Config and MutableConfig are private
	// chrome.Option are callback functions that work on Config, and they cannot
	// be compared easily without having access to Config or its Mock Interface.
	var options []chrome.Option

	if req.KeepState {
		options = append(options, chrome.KeepState())
	}

	if req.TryReuseSession {
		options = append(options, chrome.TryReuseSession())
	}

	if req.EnableHidScreenOnOobe {
		options = append(options, chrome.EnableHIDScreenOnOOBE())
	}

	if req.DontSkipOobeAfterLogin {
		options = append(options, chrome.DontSkipOOBEAfterLogin())
	}

	switch req.GetLoginMode() {
	case pb.LoginMode_LOGIN_MODE_NO_LOGIN:
		options = append(options, chrome.NoLogin())
	case pb.LoginMode_LOGIN_MODE_FAKE_LOGIN:
		options = append(options, chrome.FakeLogin(toCreds(req.Credentials)))
	case pb.LoginMode_LOGIN_MODE_GAIA_LOGIN:
		options = append(options, chrome.GAIALogin(toCreds(req.Credentials)))
	case pb.LoginMode_LOGIN_MODE_GUEST_LOGIN:
		options = append(options, chrome.GuestLogin())
	default:
		options = append(options, chrome.FakeLogin(defaultCreds))
	}

	if len(req.ExtraArgs) > 0 {
		options = append(options, chrome.ExtraArgs(req.ExtraArgs...))
	}

	if len(req.EnableFeatures) > 0 {
		for _, feature := range req.EnableFeatures {
			if feature == "LacrosOnly" {
				return nil, errors.Errorf("To enable Lacros, define `lacros` field in request, but got: [%v]", req)
			}
		}
		options = append(options, chrome.EnableFeatures(req.EnableFeatures...))
	}

	if len(req.DisableFeatures) > 0 {
		options = append(options, chrome.DisableFeatures(req.DisableFeatures...))
	}

	if len(req.LacrosExtraArgs) > 0 {
		options = append(options, chrome.LacrosExtraArgs(req.LacrosExtraArgs...))
	}

	if len(req.LacrosEnableFeatures) > 0 {
		options = append(options, chrome.LacrosEnableFeatures(req.LacrosEnableFeatures...))
	}

	if len(req.LacrosDisableFeatures) > 0 {
		options = append(options, chrome.LacrosDisableFeatures(req.LacrosDisableFeatures...))
	}

	if len(req.SigninProfileTestExtensionId) > 0 {
		options = append(options, chrome.LoadSigninProfileExtension(req.SigninProfileTestExtensionId))
	}

	if len(req.UnpackedExtensions) > 0 {
		for _, extDir := range req.UnpackedExtensions {
			options = append(options, chrome.UnpackedExtension(extDir))
		}
	}

	if len(req.LacrosUnpackedExtensions) > 0 {
		for _, extDir := range req.LacrosUnpackedExtensions {
			options = append(options, chrome.LacrosUnpackedExtension(extDir))
		}
	}

	return options, nil
}

func toCreds(c *pb.NewRequest_Credentials) chrome.Creds {
	return chrome.Creds{
		User:       c.Username,
		Pass:       c.Password,
		GAIAID:     c.GaiaId,
		Contact:    c.Contact,
		ParentUser: c.ParentUsername,
		ParentPass: c.ParentPassword,
	}
}

func toLacrosConfig(req *pb.NewRequest) (lcfg *lacrosfixt.Config, err error) {
	if req.GetLacros() == nil {
		return nil, errors.Errorf("lacros should be set in NewRequest: %v", req)
	}

	var selection lacros.Selection
	switch req.GetLacros().GetSelection() {
	case pb.Lacros_SELECTION_UNSPECIFIED:
		selection = lacros.NotSelected
	case pb.Lacros_SELECTION_ROOTFS:
		selection = lacros.Rootfs
	case pb.Lacros_SELECTION_OMAHA:
		selection = lacros.Omaha
	default:
		return nil, errors.Errorf("unsupported selection: %v", req.GetLacros().GetSelection())
	}

	lcfg = lacrosfixt.NewConfig(lacrosfixt.Selection(selection), lacrosfixt.KeepAlive(req.LacrosKeepAlive))
	return lcfg, nil
}
