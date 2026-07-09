// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package managedclientcert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/local/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/errors"
)

// ExpectedIssuer is the issuer common name of the managed client certificate.
// It is checked against both the connectors-internals Info.Issuer and the
// Chaps / PKCS#11 user token, so it is shared across every verification
// surface below rather than scoped to just one of them.
const ExpectedIssuer = "Chrome Enterprise CA"

// --- chrome://connectors-internals ---

// MCCURL is the connectors-internals managed client certificate subpage. The
// URL fragment selects that section directly.
const MCCURL = "chrome://connectors-internals/#managed-client-certificate"

// Info mirrors the object returned by InfoJS.
type Info struct {
	PolicyLevels  string `json:"policyLevels"`
	HasIdentity   bool   `json:"hasIdentity"`
	IdentityName  string `json:"identityName"`
	TrustLevel    string `json:"trustLevel"`
	KeyType       string `json:"keyType"`
	PublicKeyHash string `json:"publicKeyHash"`
	HasSSLKey     string `json:"hasSslKey"`
	Subject       string `json:"subject"`
	Issuer        string `json:"issuer"`
	SerialNumber  string `json:"serialNumber"`
}

// InfoJS pierces the connectors-internals shadow roots and returns the
// Managed Client Certificate identity, read by label text.
const InfoJS = `(function() {
  const empty = {policyLevels: '', hasIdentity: false};
  const app = document.querySelector('connectors-internals-app');
  if (!app || !app.shadowRoot) return empty;
  const tabs = app.shadowRoot.querySelector('connectors-tabs');
  if (!tabs || !tabs.shadowRoot) return empty;
  const mcc = tabs.shadowRoot.querySelector('managed-client-certificate');
  if (!mcc || !mcc.shadowRoot) return empty;
  const root = mcc.shadowRoot;

  const policyEl = root.querySelector('#policy-enabled-levels');
  const policyLevels = policyEl ? policyEl.textContent.trim() : '';

  const identities = root.querySelector('#managed-identities');
  const first = identities ? identities.querySelector('div') : null;
  if (!first) return {policyLevels: policyLevels, hasIdentity: false};

  const fields = {};
  first.querySelectorAll('div').forEach(function(d) {
    const span = d.querySelector('.bold');
    if (!span || !d.firstChild) return;
    const label = d.firstChild.textContent.replace(/:\s*$/, '').trim();
    fields[label] = span.textContent.trim();
  });

  return {
    policyLevels: policyLevels,
    hasIdentity: true,
    identityName: fields['Profile Identity Name'] || '',
    trustLevel: fields['Key Trust Level'] || '',
    keyType: fields['Key Type'] || '',
    publicKeyHash: fields['Public Key Hash'] || '',
    hasSslKey: fields['Has SSL Key'] || '',
    subject: fields['Subject'] || '',
    issuer: fields['Issuer'] || '',
    serialNumber: fields['Serial Number'] || '',
  };
})()`

// ReadInfo forces a fresh cross-document reload of the connectors-internals
// managed client certificate page on conn and returns the parsed Info. The page
// snapshots the certificate state at load time, so a caller polling for a state
// change must call this on every attempt to get a fresh read.
//
// conn is expected to already be at MCCURL. Because MCCURL carries a URL
// fragment, navigating straight to it again is a same-document no-op and Chrome
// won't reload; this bounces through about:blank first to force a true reload.
func ReadInfo(ctx context.Context, conn *chrome.Conn) (Info, error) {
	if err := conn.Navigate(ctx, "about:blank"); err != nil {
		return Info{}, errors.Wrap(err, "failed to navigate to about:blank")
	}
	if err := conn.Navigate(ctx, MCCURL); err != nil {
		return Info{}, errors.Wrap(err, "failed to reload connectors-internals")
	}
	var info Info
	if err := conn.Eval(ctx, InfoJS, &info); err != nil {
		return Info{}, errors.Wrap(err, "failed to read connectors-internals page")
	}
	return info, nil
}

// --- TLS client-certificate authentication ---

// ClientAuthSuccessText marks a successful TLS client authentication on the page.
const ClientAuthSuccessText = "Authentication OK"

// StartClientAuthServer starts a local HTTPS server that requires a TLS client
// certificate. It can't verify the cert's chain (it doesn't have the real
// enterprise CA), so it accepts any client cert and checks the issuer itself
// in the handler, writing ClientAuthSuccessText when it matches ExpectedIssuer.
func StartClientAuthServer() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 && r.TLS.PeerCertificates[0].Issuer.CommonName == ExpectedIssuer {
			fmt.Fprintln(w, ClientAuthSuccessText)
		}
	})

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{
		ClientAuth: tls.RequireAnyClientCert,
	}
	server.StartTLS()
	return server
}

// OpenClientAuth opens a new tab and navigates it to website, the local server
// that requires TLS client-certificate authentication. The returned connection
// is left open; the caller must Close it, and then decide (via quiescence and
// the page HTML) whether client authentication succeeded.
//
// It sets window.location instead of using chrome.NewConn's navigation because
// NewConn cannot finish loading the page until a client certificate has been
// selected and would hang on that.
func OpenClientAuth(ctx context.Context, cr *chrome.Chrome, website *httptest.Server) (*chrome.Conn, error) {
	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open a new tab")
	}
	if err := conn.Eval(ctx, "window.location.href = '"+website.URL+"';", nil); err != nil {
		conn.Close()
		return nil, errors.Wrapf(err, "failed to navigate to %s", website.URL)
	}
	return conn, nil
}

// --- Chaps / PKCS#11 user token ---

// FindUserTokenCert waits for the Chaps / PKCS#11 user token belonging to
// username and returns the first certificate in it whose issuer common name is
// issuer, or nil if the token contains no such certificate. A nil certificate
// with a nil error means the certificate is provably absent; callers decide
// whether presence or absence is the expected outcome.
func FindUserTokenCert(ctx context.Context, username, issuer string) (*x509.Certificate, error) {
	r := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create an hwsec helper")
	}
	cc := helper.CryptohomeClient()
	chaps, err := pkcs11.NewChaps(ctx, r, cc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a Chaps client")
	}

	if err := cc.WaitForUserToken(ctx, username); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the user token")
	}
	userSlot, err := cc.GetTokenForUser(ctx, username)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the user token slot")
	}

	certs, err := chaps.ListCerts(ctx, userSlot)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list certificates in the user token")
	}
	for _, ci := range certs {
		if c := ci.Cert(); c != nil && c.Issuer.CommonName == issuer {
			return c, nil
		}
	}
	return nil, nil
}
