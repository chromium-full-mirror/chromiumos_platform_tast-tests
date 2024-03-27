// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// util.go contains convenience the util funcs that manage interactions with proxy in the test environment.

// ConfigureChrome sets up chrome for mitmproxy.
// User should call this function to update the network after a mitmproxy server is started.
// User should also call a returned reset closure to clean up the configuration when the proxy is no longer in use.
func ConfigureChrome(ctx context.Context, p Proxy, cr *chrome.Chrome) (func(context.Context, *chrome.Chrome) error, error) {
	testing.ContextLog(ctx, "proxyutil: setting up chrome")
	reset := func(ctx context.Context, cr *chrome.Chrome) error {
		var err error
		if err := clearChromeProxy(ctx, cr); err != nil {
			err = errors.Wrap(err, "failed to reset proxy in chrome")
		}
		if err := removeRootCertFromNSS(ctx, cr); err != nil {
			err = errors.Wrap(err, "failed to remove root certificate in user home")
		}
		if err != nil {
			testing.ContextLog(ctx, "proxyutil: cleaned up chrome, err: ", err)
		} else {
			testing.ContextLog(ctx, "proxyutil: cleaned up chrome successfully")
		}
		return err
	}

	// Copy a root CA certificate to user home
	if err := importRootCertToNSS(ctx, p, cr); err != nil {
		reset(ctx, cr)
		return nil, errors.Wrap(err, "failed to import root certificate from proxy to chrome")
	}
	// Configure an IP of the mitmproxy server in chrome
	if err := setChromeProxy(ctx, cr, p.ProxyAddress()); err != nil {
		reset(ctx, cr)
		return nil, errors.Wrap(err, "failed to configure proxy in chrome")
	}
	return reset, nil
}

func setChromeProxy(ctx context.Context, cr *chrome.Chrome, proxyAddress string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	host, port, err := parseProxyAddress(proxyAddress)
	if err != nil {
		return err
	}

	var settingsAPICall = fmt.Sprintf(`
       var config = {
               mode: 'fixed_servers',
               rules: {
                       singleProxy: {
                               host: %q,
                               port: %d
                       }
               }
       };
       chrome.proxy.settings.set(
                       {value: config, scope: 'regular'},
                       function() {})`, host, port)

	return tconn.Eval(ctx, settingsAPICall, nil)
}

// getChromeProxy gets proxy in Chrome by calling tconn API.
func getChromeProxy(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}
	var data interface{}
	err = tconn.Eval(ctx,
		`chrome.proxy.settings.get(
	                       {'incognito': false},
	                       function(config) { console.log(config) })`, &data)
	testing.ContextLogf(ctx, "proxyutil: jsProxy: %v, err: %v", data, err)
	return err
}

// clearChromeProxy unsets network proxy by calling tconn API.
func clearChromeProxy(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}
	var settingsAPICall = `chrome.proxy.settings.clear({scope:'regular'}, function() {});`
	return tconn.Eval(ctx, settingsAPICall, nil)
}

// parseProxyAddress parses a host:port string and returns the components.
func parseProxyAddress(proxyAddress string) (host string, port int, err error) {
	parts := strings.Split(proxyAddress, ":")
	if len(parts) != 2 {
		return "", 0, errors.Errorf("got invalid proxy address %q", proxyAddress)
	}

	port, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, errors.Errorf("got invalid port int in %q", proxyAddress)
	}
	return parts[0], port, nil
}

// importRootCertToNSS adds the proxy root certificate to nssdb for the current Chrome user.
func importRootCertToNSS(ctx context.Context, p Proxy, cr *chrome.Chrome) error {
	userHome, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	// Find the root certificate that the mitmproxy uses.
	certFile, err := p.RootCertificate(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to download root certificate")
	}

	// Save it to nssdb to let Chrome trust.
	cmd := testexec.CommandContext(ctx, "certutil", "-d", fmt.Sprintf("sql:%s/.pki/nssdb", userHome),
		"-A", "-t", "C,C,C", "-n", "test.proxy", "-i", certFile)
	testing.ContextLog(ctx, "proxyutil: import root cert, cmd: ", cmd)
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrap(err, "failed to import certificate")
	}
	return nil
}

// removeRootCertFromNSS removes the proxy root certificate from nssdb for the current Chrome user.
func removeRootCertFromNSS(ctx context.Context, cr *chrome.Chrome) error {
	userHome, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	cmd := testexec.CommandContext(ctx, "certutil", "-d", fmt.Sprintf("sql:%s/.pki/nssdb", userHome),
		"-D", "-n", "test.proxy")
	testing.ContextLog(ctx, "proxyutil: remove root cert, cmd: ", cmd)
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrap(err, "failed to remove certificate")
	}
	return nil
}
