// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/proxy"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/errors"
)

// SetProxy sets the network proxy by calling tconn API.
// It is hardcoded to use fixed_servers mode for now.
func (c *Chrome) SetProxy(ctx context.Context, proxyAddress string) error {
	tconn, err := c.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	host, port, err := parseProxyAddress(proxyAddress)
	if err != nil {
		return err
	}

	var settingsAPICall = fmt.Sprintf(`
	var config = {
		mode: "fixed_servers",
		rules: {
			singleProxy: {
				host: %q,
				port:%d
			}
		}
	};
	chrome.proxy.settings.set(
			{value: config, scope: 'regular'},
			function() {})`, host, port)

	return tconn.Eval(ctx, settingsAPICall, nil)
}

// UnsetProxy unsets network proxy by calling tconn API.
func (c *Chrome) UnsetProxy(ctx context.Context) error {
	tconn, err := c.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	var settingsAPICall = `chrome.proxy.settings.clear({scope:'regular'}, function() {});`

	return tconn.Eval(ctx, settingsAPICall, nil)
}

// importRootCertificate imports the proxy root certificate to current Chrome user.
func (c *Chrome) importRootCertificate(ctx context.Context, proxy proxy.Proxy) error {
	if !proxy.IsRunning() {
		return errors.New("proxy is not started")
	}

	userHome, err := cryptohome.UserPath(ctx, c.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	certFile, err := proxy.RootCertificate(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to download root certificate")
	}

	cmd := testexec.CommandContext(ctx, "certutil", "-d", fmt.Sprintf("sql:%s/.pki/nssdb", userHome),
		"-A", "-t", "C,C,C", "-n", "test.proxy", "-i", certFile)
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrap(err, "failed to import certificate")
	}
	return nil
}

// removeRootCertificate removes the proxy root certificate from the current Chrome user.
func (c *Chrome) removeRootCertificate(ctx context.Context) error {
	userHome, err := cryptohome.UserPath(ctx, c.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	cmd := testexec.CommandContext(ctx, "certutil", "-d", fmt.Sprintf("sql:%s/.pki/nssdb", userHome),
		"-D", "-n", "test.proxy")
	if err := cmd.Run(); err != nil {
		cmd.DumpLog(ctx)
		return errors.Wrap(err, "failed to remove certificate")
	}
	return nil

}

// LaunchAndApplyProxy launches defined proxy and apply to current Chrome.
// It returns cleanup function and error if present.
func (c *Chrome) LaunchAndApplyProxy(ctx context.Context, proxy proxy.Proxy) (func(context.Context) error, error) {
	if !proxy.IsRunning() {
		if err := proxy.Start(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to start proxy")
		}
	}

	if err := c.importRootCertificate(ctx, proxy); err != nil {
		return nil, errors.Wrap(err, "failed to import CA certificate")
	}

	cleanup := func(ctx context.Context) error {
		err1 := c.UnsetProxy(ctx)
		err2 := proxy.Close(ctx)
		err3 := c.removeRootCertificate(ctx)
		returnErr := errors.Join(err1, err2, err3)
		if returnErr != nil {
			return errors.Wrap(returnErr, "failed to cleanup proxy")
		}
		return nil
	}

	return cleanup, c.SetProxy(ctx, proxy.ProxyAddress())
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

// NewChromeWithProxy new Chrome with proxy.
// Please note that for in-session mode, it's better to use LaunchAndApplyProxy.
// Method returns cleanup method which will close cr returned.
// Therefore, there is no need to cr.Close.
// TODO(b/301880537): avoid using --ignore-certificate-errors when applying proxy testing.
func NewChromeWithProxy(ctx context.Context, proxy proxy.Proxy, opts ...Option) (*Chrome, func(context.Context) error, error) {
	if !proxy.IsRunning() {
		if err := proxy.Start(ctx); err != nil {
			return nil, nil, errors.Wrap(err, "failed to start proxy")
		}
	}

	// Set up proxy sever.
	opts = append(opts, ExtraArgs(fmt.Sprintf("--proxy-server=%s", proxy.ProxyAddress())))
	// Ignore certificate errors then Chrome doesn't need to trust cert.
	opts = append(opts, ExtraArgs("--ignore-certificate-errors"))

	cr, err := New(ctx, opts...)
	if err != nil {
		proxy.Close(ctx)
		return nil, nil, errors.Wrap(err, "failed to create a chrome")
	}

	cleanup := func(ctx context.Context) error {
		err1 := proxy.Close(ctx)
		err2 := cr.Close(ctx)
		returnErr := errors.Join(err1, err2)
		if returnErr != nil {
			return errors.Wrap(returnErr, "failed to cleanup proxy")
		}
		return nil
	}

	return cr, cleanup, nil
}
