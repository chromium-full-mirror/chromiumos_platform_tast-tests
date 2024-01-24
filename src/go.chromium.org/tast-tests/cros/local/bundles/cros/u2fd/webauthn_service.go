// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package u2fd

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/u2fd"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	localu2fd "go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
	attestationType   hwsec.AttestationType
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
	srv      *localu2fd.WebAuthnHTTPServer

	fakeDMS    *fakedms.FakeDMS
	fakeDMSDir string

	cfg      webauthnConfig
	password string
}

func (c *WebauthnService) New(ctx context.Context, req *hwsec.NewRequest) (*empty.Empty, error) {
	ok := false

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return nil, errors.Wrap(err, "failed to restart ui job")
	}

	srv := localu2fd.NewWebAuthnHTTPServer(ctx, http.Dir(req.GetDataPath()))
	defer func(ctx context.Context) {
		if !ok {
			srv.Close(ctx)
		}
	}(ctxForCleanUp)

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
	defer func(ctx context.Context) {
		if !ok {
			if err := keyboard.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close keyboard")
			}
		}
	}(ctxForCleanUp)

	var opts []chrome.Option
	if req.GetKeepState() {
		opts = append(opts, chrome.KeepState())
	}

	if req.GetAllowEnterpriseAttestation() {
		tmpdir, err := ioutil.TempDir("", "fdms-")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create temp dir")
		}
		defer func(ctx context.Context) {
			if !ok {
				if err := os.RemoveAll(tmpdir); err != nil {
					testing.ContextLogf(ctx, "Failed to delete %s: %v", tmpdir, err)
				}
				c.fakeDMSDir = ""
			}
		}(ctxForCleanUp)
		c.fakeDMSDir = tmpdir

		fdms, err := fakedms.New(c.s.ServiceContext(), tmpdir)
		if err != nil {
			return nil, errors.Wrap(err, "failed to start FakeDMS")
		}
		defer func(ctx context.Context) {
			if !ok {
				fdms.Stop(ctx)
				c.fakeDMS = nil
			}
		}(ctxForCleanUp)
		c.fakeDMS = fdms

		pb := policy.NewBlob()
		pb.AddPolicies([]policy.Policy{
			&policy.SecurityKeyPermitAttestation{Val: []string{"localhost"}},
		})
		if err := fdms.WritePolicyBlob(pb); err != nil {
			return nil, errors.Wrap(err, "failed to write policies to FakeDMS")
		}
		opts = append(opts, chrome.DMSPolicy(fdms.URL))
		opts = append(opts, chrome.FakeLogin(chrome.Creds{User: "tast-user@managedchrome.com", Pass: "testpass"}))
	}

	cr, br, closeBrowser, err := browserfixt.SetUpWithNewChrome(ctx, bt, lacrosfixt.NewConfig(), opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to log in by Chrome with %v browser", bt)
	}
	defer func(ctx context.Context) {
		if !ok {
			if err := closeBrowser(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close browser")
			}
			if err := cr.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close Chrome")
			}
		}
	}(ctxForCleanUp)

	conn, err := br.NewConn(ctx, srv.URL+"/webauthn.html")
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to test website")
	}
	defer func(ctx context.Context) {
		if !ok {
			if err := conn.Close(); err != nil {
				testing.ContextLog(ctx, "Failed to close connection")
			}
		}
	}(ctxForCleanUp)

	c.cr = cr
	c.br = br
	c.closeBrowser = closeBrowser
	c.keyboard = keyboard
	c.conn = conn
	c.srv = srv

	ok = true
	return &empty.Empty{}, nil
}

func (c *WebauthnService) Close(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var lastErr error

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close connection: ", err)
			lastErr = err
		}
		c.conn = nil
	}
	if c.closeBrowser != nil {
		if err := c.closeBrowser(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close browser: ", err)
			lastErr = err
		}
		c.br = nil
	}
	if c.cr != nil {
		if err := c.cr.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close Chrome: ", err)
			lastErr = err
		}
		c.cr = nil
	}
	if c.keyboard != nil {
		if err := c.keyboard.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close keyboard: ", err)
			lastErr = err
		}
		c.keyboard = nil
	}
	if c.fakeDMS != nil {
		c.fakeDMS.Stop(ctx)
		c.fakeDMS = nil
	}
	if c.fakeDMSDir != "" {
		if err := os.RemoveAll(c.fakeDMSDir); err != nil {
			testing.ContextLogf(ctx, "Failed to delete %s: %v", c.fakeDMSDir, err)
			lastErr = err
		}
		c.fakeDMSDir = ""
	}
	if c.srv != nil {
		c.srv.Close(ctx)
		c.srv = nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return &empty.Empty{}, nil
}

func (c *WebauthnService) StartWebauthn(ctx context.Context, req *hwsec.StartWebauthnRequest) (*empty.Empty, error) {
	c.cfg = webauthnConfig{
		userVerification:  req.GetUserVerification(),
		authenticatorType: req.GetAuthenticatorType(),
		attestationType:   req.GetAttestationType(),
		hasDialog:         req.GetHasDialog(),
	}
	return &empty.Empty{}, nil
}

func (c *WebauthnService) StartMakeCredential(ctx context.Context, req *empty.Empty) (*hwsec.WebAuthnCredential, error) {
	config := u2fd.WebAuthnRegistrationConfig{
		Uv: uvToString(c.cfg.userVerification),
	}
	fillAuthenticatorAttachment(&config, c.cfg.authenticatorType)
	fillAttestation(&config, c.cfg.attestationType)
	channel := localu2fd.InitiateMakeCredentialInLocalSite(ctx, c.conn, config)

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
	if res.Cred == nil {
		return nil, errors.New("failed to retrieve created WebAuthn credential")
	}

	cred := hwsec.WebAuthnCredential{
		CredentialIdB64: res.Cred.CredentialIDB64,
		PublicKey: &hwsec.PublicKey{
			DataB64: res.Cred.PublicKey.DataB64,
			KeyType: res.Cred.PublicKey.KeyType,
		},
		SerialNumberB64: res.Cred.SerialNumberB64,
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
		if err := localu2fd.ChoosePlatformAuthenticator(ctx, tconn); err != nil {
			return nil, err
		}
	}

	if c.cfg.hasDialog {
		if err := localu2fd.WaitForWebAuthnDialog(ctx, tconn); err != nil {
			return nil, err
		}
	} else {
		if err := localu2fd.WaitForPopup(ctx, tconn); err != nil {
			return nil, err
		}
	}

	return &empty.Empty{}, nil
}

func (c *WebauthnService) EndMakeCredential(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test API connection")
	}

	if !c.cfg.hasDialog {
		if err := localu2fd.WaitUntilPopupGone(ctx, tconn); err != nil {
			return nil, err
		}
	}

	return &empty.Empty{}, nil
}

func (c *WebauthnService) StartGetAssertion(ctx context.Context, req *hwsec.StartGetAssertionRequest) (*empty.Empty, error) {
	cred := req.GetCred()
	if cred == nil {
		return nil, errors.New("failed to parse WebAuthn credential from request")
	}
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
	channel := localu2fd.InitiateGetAssertionInLocalSite(ctx, c.conn, config)

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
		if err := localu2fd.WaitForWebAuthnDialog(ctx, tconn); err != nil {
			return nil, err
		}
	} else {
		if err := localu2fd.WaitForPopup(ctx, tconn); err != nil {
			return nil, err
		}
	}

	return &empty.Empty{}, nil
}

func (c *WebauthnService) EndGetAssertion(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	tconn, err := c.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get test API connection")
	}

	if !c.cfg.hasDialog {
		if err := localu2fd.WaitUntilPopupGone(ctx, tconn); err != nil {
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

func fillAttestation(config *u2fd.WebAuthnRegistrationConfig, t hwsec.AttestationType) {
	// Ignore "NOT_SPECIFIED" and unknown types.
	switch t {
	case hwsec.AttestationType_NONE:
		config.Attestation = "none"
	case hwsec.AttestationType_INDIRECT:
		config.Attestation = "indirect"
	case hwsec.AttestationType_DIRECT:
		config.Attestation = "direct"
	case hwsec.AttestationType_ENTERPRISE:
		config.Attestation = "enterprise"
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
