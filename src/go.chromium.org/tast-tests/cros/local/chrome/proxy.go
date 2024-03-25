// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy/mitmproxy"
	"go.chromium.org/tast/core/errors"
)

// setProxy sets the network proxy by calling tconn API.
// It is hardcoded to use fixed_servers mode for now.
func (c *Chrome) setProxy(ctx context.Context, proxyAddress string) error {
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

// unsetProxy unsets network proxy by calling tconn API.
func (c *Chrome) unsetProxy(ctx context.Context) error {
	tconn, err := c.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	var settingsAPICall = `chrome.proxy.settings.clear({scope:'regular'}, function() {});`

	return tconn.Eval(ctx, settingsAPICall, nil)
}

// importRootCertificate imports the proxy root certificate to current Chrome user.
func (c *Chrome) importRootCertificate(ctx context.Context) error {
	userHome, err := cryptohome.UserPath(ctx, c.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	certFile, err := c.proxy.RootCertificate(ctx)
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

// LaunchAndApplyProxy launches proxy and apply to current Chrome.
// If the test calls chrome.Close() in the end, then we don't need to cleanup proxy separately.
func (c *Chrome) LaunchAndApplyProxy(ctx context.Context, proxy proxy.Proxy) error {
	if proxy == nil {
		return errors.New("Proxy cannot be nil")
	}

	c.proxy = proxy

	if !proxy.IsRunning() {
		if err := proxy.Start(ctx); err != nil {
			return errors.Wrap(err, "failed to start proxy")
		}
	}

	if err := c.importRootCertificate(ctx); err != nil {
		return errors.Wrap(err, "failed to import CA certificate")
	}

	if err := c.setProxy(ctx, proxy.ProxyAddress()); err != nil {
		return errors.Wrap(err, "fail to setup proxy address")
	}

	return nil
}

// CleanupProxy will cleanup proxy.
// It will be called in chrome.Close().
// Therefore, user doesn't need to call it in most scenario.
func (c *Chrome) CleanupProxy(ctx context.Context) error {
	if c.proxy != nil {
		err1 := c.unsetProxy(ctx)
		err2 := c.proxy.Close(ctx)
		err3 := c.removeRootCertificate(ctx)
		c.proxy = nil
		returnErr := errors.Join(err1, err2, err3)
		if returnErr != nil {
			return errors.Wrap(returnErr, "failed to cleanup proxy")
		}
	}
	return nil
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

// CreateLaunchAndApplyProxy creates and apply a new proxy when proxy.enable is true.
func (c *Chrome) CreateLaunchAndApplyProxy(ctx context.Context) proxy.Proxy {
	if proxy.IsProxyEnabled() {
		p, err := mitmproxy.New(ctx)
		if err != nil {
			panic(fmt.Sprintf("Failed to create MitmProxy: %v", err))
		}
		if err := c.LaunchAndApplyProxy(ctx, p); err != nil {
			panic(fmt.Sprintf("Failed to launch and apply proxy: %v", err))
		}
		return p
	}

	return nil
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
