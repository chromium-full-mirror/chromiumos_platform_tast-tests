// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package u2fd

import (
	"context"
	"net/http"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/u2fd"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/services/cros/hwsec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			hwsec.RegisterWebauthnServiceServer(srv, &WebauthnService{s: s})
		},
	})
}

type webauthnConfig struct {
	userVerification  hwsec.UserVerification
	authenticatorType hwsec.AuthenticatorType
	hasDialog         bool
}

// WebauthnService implements tast.cros.hwsec.WebauthnService.
type WebauthnService struct {
	s *testing.ServiceState

	cr           *chrome.Chrome
	br           *browser.Browser
	closeBrowser uiauto.Action
	// Keeping keyboard in state instead of creating it each time because it takes about 5 seconds to create a keyboard.
	keyboard *input.KeyboardEventWriter
	conn     *chrome.Conn
	srv      *u2fd.WebAuthnHTTPServer

	cfg      webauthnConfig
	password string
}

func (c *WebauthnService) New(ctx context.Context, req *hwsec.NewRequest) (*empty.Empty, error) {
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return nil, errors.Wrap(err, "failed to restart ui job")
	}

	c.srv = u2fd.NewWebAuthnHTTPServer(ctx, http.Dir(req.GetDataPath()))

	var bt browser.Type
	if req.GetBrowserType() == hwsec.BrowserType_ASH {
		bt = browser.TypeAsh
	} else {
		bt = browser.TypeLacros
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get keyboard")
	}

	var opts []chrome.Option
	if req.GetKeepState() {
		opts = append(opts, chrome.KeepState())
	}

	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(), opts...)
	if err != nil {
		keyboard.Close()
		return nil, errors.Wrapf(err, "failed to log in by Chrome with %v browser", bt)
	}
	conn, err := br.NewConn(ctx, c.srv.URL+"/webauthn.html")
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to test website")
	}
	c.keyboard = keyboard
	c.cr = cr
	c.br = br
	c.closeBrowser = closeBrowser
	c.conn = conn

	return &empty.Empty{}, nil
}

func (c *WebauthnService) Close(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	if c.closeBrowser != nil {
		c.closeBrowser(ctx)
		c.br = nil
	}
	if c.cr != nil {
		c.cr.Close(ctx)
		c.cr = nil
	}
	if c.keyboard != nil {
		c.keyboard.Close()
		c.keyboard = nil
	}
	if c.srv != nil {
		c.srv.Close(ctx)
		c.srv = nil
	}
	return &empty.Empty{}, nil
}

func (c *WebauthnService) StartWebauthn(ctx context.Context, req *hwsec.StartWebauthnRequest) (*empty.Empty, error) {
	c.cfg = webauthnConfig{
		userVerification:  req.GetUserVerification(),
		authenticatorType: req.GetAuthenticatorType(),
		hasDialog:         req.GetHasDialog(),
	}
	return &empty.Empty{}, nil
}

func (c *WebauthnService) StartMakeCredential(ctx context.Context, req *empty.Empty) (*hwsec.WebAuthnCredential, error) {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test API connection")
	}

	if !c.cfg.hasDialog {
		if err := u2fd.WaitUntilPopupGone(ctx, tconn); err != nil {
			return nil, err
		}
	}

	config := u2fd.WebAuthnRegistrationConfig{
		Attestation: "none",
		Uv:          uvToString(c.cfg.userVerification),
	}
	fillAuthenticatorAttachment(&config, c.cfg.authenticatorType)
	channel := u2fd.InitiateMakeCredentialInLocalSite(ctx, c.conn, config)

	var res u2fd.MakeCredentialResult
	select {
	case res = <-channel:
		break
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if res.Err != nil {
		return nil, res.Err
	}

	cred := hwsec.WebAuthnCredential{
		CredentialIdB64: res.Cred.CredentialIDB64,
		PublicKey: &hwsec.PublicKey{
			DataB64: res.Cred.PublicKey.DataB64,
			KeyType: res.Cred.PublicKey.KeyType,
		},
	}
	return &cred, nil
}

func (c *WebauthnService) DoMakeCredential(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test API connection")
	}

	// If authenticator type is "Platform", there's only platform option so
	// we don't have to manually click "This device".
	if c.cfg.authenticatorType != hwsec.AuthenticatorType_PLATFORM {
		if err := u2fd.ChoosePlatformAuthenticator(ctx, tconn); err != nil {
			return nil, err
		}
	}

	if c.cfg.hasDialog {
		if err := u2fd.WaitForWebAuthnDialog(ctx, tconn); err != nil {
			return nil, err
		}
	} else {
		if err := u2fd.WaitForPopup(ctx, tconn); err != nil {
			return nil, err
		}
	}

	return &empty.Empty{}, nil
}

func (c *WebauthnService) StartGetAssertion(ctx context.Context, req *hwsec.StartGetAssertionRequest) (*empty.Empty, error) {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test API connection")
	}

	if !c.cfg.hasDialog {
		if err := u2fd.WaitUntilPopupGone(ctx, tconn); err != nil {
			return nil, err
		}
	}

	cred := req.GetCred()
	config := u2fd.WebAuthnAssertionConfig{
		Keys: []u2fd.WebAuthnCredential{
			{
				CredentialIDB64: cred.CredentialIdB64,
				PublicKey: u2fd.PublicKey{
					DataB64: cred.PublicKey.DataB64,
					KeyType: cred.PublicKey.KeyType,
				},
			},
		},
		Uv: uvToString(c.cfg.userVerification),
	}
	channel := u2fd.InitiateGetAssertionInLocalSite(ctx, c.conn, config)

	select {
	case err := <-channel:
		if err != nil {
			return nil, err
		}
		return &empty.Empty{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *WebauthnService) DoGetAssertion(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test API connection")
	}

	if c.cfg.hasDialog {
		if err := u2fd.WaitForWebAuthnDialog(ctx, tconn); err != nil {
			return nil, err
		}
	} else {
		if err := u2fd.WaitForPopup(ctx, tconn); err != nil {
			return nil, err
		}
	}

	return &empty.Empty{}, nil
}

func (c *WebauthnService) EnterPassword(ctx context.Context, req *hwsec.EnterPasswordRequest) (*empty.Empty, error) {
	if err := c.keyboard.Type(ctx, req.GetPassword()+"\n"); err != nil {
		return nil, errors.Wrap(err, "failed to type password into ChromeOS auth dialog")
	}
	return &empty.Empty{}, nil
}

func fillAuthenticatorAttachment(config *u2fd.WebAuthnRegistrationConfig, t hwsec.AuthenticatorType) {
	// Ignore "UNSPECIFIED" and unknown types.
	switch t {
	case hwsec.AuthenticatorType_CROSS_PLATFORM:
		config.AuthenticatorAttachment = "cross-platform"
	case hwsec.AuthenticatorType_PLATFORM:
		config.AuthenticatorAttachment = "platform"
	}
}

func uvToString(uv hwsec.UserVerification) string {
	switch uv {
	case hwsec.UserVerification_DISCOURAGED:
		return "discouraged"
	case hwsec.UserVerification_PREFERRED:
		return "preferred"
	case hwsec.UserVerification_REQUIRED:
		return "required"
	}
	// Fallback to "preferred", the default state.
	return "preferred"
}
