// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// util.go contains convenience the util funcs that manage interactions with proxy in the test environment.

var caRootVar = testing.RegisterVarString(
	"proxy.ca_certificate_credential",
	"",
	"CA used by mitmproxy",
)

var caPrivateVar = testing.RegisterVarString(
	"proxy.ca_certificate_private_key",
	"",
	"Private CA used by mitmproxy",
)

// CaHashcodeVar returns the hashcode of public key.
var CaHashcodeVar = testing.RegisterVarString(
	"proxy.ca_certificate_hashcode",
	"",
	"Hashcode of CA",
)

func generateCustomCA(path, name string, privateKey bool) error {
	filePath := filepath.Join(path, name)

	// Create or overwrite the file
	file, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer file.Close() // Ensure file is closed properly

	var keys []string
	// Private key should be the first part if we want to include it.
	if privateKey {
		if caPrivateVar.Value() == "" {
			return errors.New("private key is empty")
		}

		keys = append(keys, caPrivateVar.Value())
	}

	if caRootVar.Value() == "" {
		return errors.New("public key is empty")
	}
	// Always contains public key.
	keys = append(keys, caRootVar.Value())

	content := strings.Join(keys, "")
	// Write content to the file.
	if _, err = file.WriteString(content); err != nil {
		return errors.Wrap(err, "failed to write to ca")
	}

	return nil
}

// ConfigureChrome sets up mitmproxy for Chrome in-session.
// TODO(b/325270550): Remove this util function once it replaces all the code with Connect.
func ConfigureChrome(ctx context.Context, p Proxy, cr *chrome.Chrome) (func(context.Context, *chrome.Chrome) error, error) {
	err := p.Connect(ctx, cr)
	return func(context.Context, *chrome.Chrome) error { return nil }, err
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
func importRootCertToNSS(ctx context.Context, certPath string, cr *chrome.Chrome) error {
	if certPath == "" {
		return errors.New("cert path can't be empty")
	}

	userHome, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user path")
	}

	// Save it to nssdb to let Chrome trust.
	cmd := testexec.CommandContext(ctx, "certutil", "-d", fmt.Sprintf("sql:%s/.pki/nssdb", userHome),
		"-A", "-t", "C,C,C", "-n", "test.proxy", "-i", certPath)
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

func isConnectedForURL(ctx context.Context, proxyAddr, url string) (bool, error) {
	state, err := lookupProxyForURL(ctx, url)
	if err != nil {
		return false, errors.Wrap(err, "failed to look up proxy info")
	}
	lookupRE := regexp.MustCompile("PROXY " + proxyAddr) // "PROXY x.x.x.x(:<port>)?"
	return lookupRE.Match([]byte(state)), nil
}

// lookupProxyForURL returns a string of proxy server info in the PAC format from the system for the given URL.
// Example:
//
//	"PROXY 100.115.92.xxx:xxxx"
//	"DIRECT" (when no proxy is set by default)
func lookupProxyForURL(ctx context.Context, url string) (string, error) {
	const (
		dbusName   = "org.chromium.NetworkProxyService"
		dbusPath   = "/org/chromium/NetworkProxyService"
		dbusMethod = "org.chromium.NetworkProxyServiceInterface.ResolveProxy"
	)
	_, obj, err := dbusutil.Connect(ctx, dbusName, dbus.ObjectPath(dbusPath))
	if err != nil {
		return "", errors.Wrapf(err, "failed to connect to %s for proxy info", dbusName)
	}
	var state string
	var errorMsg string
	if err := obj.CallWithContext(ctx, dbusMethod, 0, url).Store(&state, &errorMsg); err != nil {
		return "", errors.Wrapf(err, "failed to get the proxy, msg: %v", errorMsg)
	}
	testing.ContextLogf(ctx, "proxy: lookup: %+v", state)
	return state, nil
}
