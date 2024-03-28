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
