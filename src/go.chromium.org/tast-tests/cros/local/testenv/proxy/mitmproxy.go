// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DefaultListenPort is the default port that the proxy is listening.
	DefaultListenPort = 4040
	// DefaultBinaryPath is the default binary path to be used.
	DefaultBinaryPath = "/usr/local/bin/mitmdump"
)

const (
	// It only contains the certificate.
	// It will be installed in browser, arc, etc.
	defaultPubCertFile = "mitmproxy-ca-cert.pem"
	// It contains both the certificate and private key.
	// It is required by mitmproxy itself.
	defaultKeyPairCertFile = "mitmproxy-ca.pem"
	defaultConfDir         = "/usr/local/tmp/mitmproxy"

	// SystemCert refers to the system CA cert trusted by Chrome by default.
	SystemCert = "system"
	// UserCert refers to the user CA cert, that can be used only in user session post login.
	UserCert = "user"
)

// MitmProxy represents a structure of mitmproxy.
type MitmProxy struct {
	binaryPath          string
	port                int
	host                string
	outDir              string
	dumpFull            bool // True only if a tcpdump-like full dump is needed, default: False
	dumpFileName        string
	dumpFilePath        string
	confDir             string
	pid                 int // pid of the proxy server process
	cmd                 *testexec.Cmd
	isRunning           bool // Is the proxy running? It is set to true on starting proxy.
	removeCert          bool // Should remove cert after test is completed.
	healthCheck         bool
	scriptPaths         []string // Addon scripts used by mitmproxy.
	options             []string // Other options provided by users. We will add --set option to command.
	lifelineFD          *os.File // Used by pathcpanel to track the lifetime of the proxy server
	dumpHTTPFlowEnabled bool
	allowedHosts        []string
	ignoredHosts        []string
	customCA            bool                        // true if the system CA cert is used
	disconnect          func(context.Context) error // Called in Close to disconnect from chrome if connected
	forceUnsetOnClose   bool
}

// NewMitmProxy creates a new MitmProxy instance with default configuration and option overrides.
func NewMitmProxy(ctx context.Context, opts ...Option) (Proxy, error) {
	mp := &MitmProxy{
		binaryPath:          DefaultBinaryPath,
		port:                DefaultListenPort,
		confDir:             defaultConfDir,
		removeCert:          true,
		healthCheck:         true,
		dumpHTTPFlowEnabled: false,
		customCA:            false,
		scriptPaths:         []string{},
		options:             []string{},
		forceUnsetOnClose:   false,
	}

	// Override any value if users pass option from test.
	for _, opt := range opts {
		if err := opt(mp); err != nil {
			return nil, err
		}
	}

	// Set OutDir for saving per-test logs and dump files.
	// OutDir could be set by the context or the `OutDir` option passed in as args. If both are set, the option takes precedence.
	// In most cases, it is just okay to use the context by default except for
	// when a local RPC server calls this func with a service-scope context that has no OutDir associated.
	// In this case, the `OutDir` option should be used explicitly.
	if mp.outDir == "" {
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok || outDir == "" {
			return nil, errors.New("OutDir should be set in the context or in the option")
		}
		mp.outDir = outDir
	}

	// Start a proxy automatically by default.
	if err := mp.start(ctx); err != nil {
		mp.Close(ctx)
		return nil, errors.Wrap(err, "failed to start mitmproxy on New")
	}
	return Proxy(mp), nil
}

// IsRunning returns whether the proxy is running.
func (mp *MitmProxy) IsRunning() bool {
	return mp.isRunning
}

// IsConnected returns whether proxy is connected to the system (ash-chrome).
func (mp *MitmProxy) IsConnected(ctx context.Context) (bool, error) {
	return isConnectedForURL(ctx, mp.ProxyAddress(), "https://google.com")
}

// start launches the mitmproxy.
func (mp *MitmProxy) start(ctx context.Context) (retErr error) {
	// Ensure that no proxy process has been running before this start.
	if err := forceKill(ctx); err != nil {
		return errors.Wrap(err, "failed to kill running mitmproxy processes")
	}

	// Clean up any existing CA certificates and create an empty conf dir before starting a proxy.
	// This makes sure that the certificate is uniquely used during this proxy session.
	if err := mp.removeCertDir(); err != nil {
		return errors.Wrap(err, "failed to delete proxy conf dir")
	}
	if err := os.MkdirAll(mp.confDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q for mitmdump config", mp.confDir)
	}
	// Clean up when it fails to Start.
	defer func() {
		if retErr != nil {
			mp.Close(ctx)
		}
	}()

	// It's crucial to add the dump HTTP flow addon as the first script.
	// MitmProxy executes addon scripts sequentially, so order directly impacts functionality.
	if mp.dumpHTTPFlowEnabled {
		path, err := mp.generateAddon("dump_http_flow", dumpHTTPFlowAddon)
		if err != nil {
			return err
		}
		mp.scriptPaths = append([]string{path}, mp.scriptPaths...)
	}

	if len(mp.allowedHosts) != 0 {
		path, err := mp.generateAddon("http_flow_filter", httpFlowFilterAddon)
		if err != nil {
			return err
		}
		mp.scriptPaths = append(mp.scriptPaths, path)
	}

	// Copy the system CA certs trusted by Chrome from the private vars if the CustomCA is set. If unset, mitmproxy creates the ones on the fly.
	if mp.customCA {
		if err := generateCustomCA(mp.confDir, defaultPubCertFile, false); err != nil {
			return errors.Wrapf(err, "failed to generate %s", defaultPubCertFile)
		}
		if err := generateCustomCA(mp.confDir, defaultKeyPairCertFile, true); err != nil {
			return errors.Wrapf(err, "failed to generate %s", defaultKeyPairCertFile)
		}
	}

	// The pid of the proxy process is required to configure the isolated network namespace in which the process runs.
	// The mitmdump process forks, resulting in two running processes. The pid addon will ensure the pid of the actual
	// proxy server process will be used to configure the network namespace.
	pidPath, err := mp.configurePidAddon()
	if err != nil {
		return errors.Wrap(err, "failed to configure pid addon")
	}

	// Create a new config file in the current config directory.
	configFilePath := filepath.Join(mp.confDir, "config.yaml")
	if err = mp.configureMitmdump(ctx, configFilePath); err != nil {
		return errors.Wrapf(err, "failed to create config file at %s", configFilePath)
	}

	// Run a proxy server process in non-interactive mode (mitmdump).
	nowStr := time.Now().Format("20060102-150405")
	logFilePath := filepath.Join(mp.outDir, fmt.Sprintf("mitmproxy_%s.log", nowStr))
	var proxyCmd string
	if mp.dumpFull {
		mp.dumpFileName = fmt.Sprintf("mitmproxy_full_%s.dump", nowStr)
		mp.dumpFilePath = filepath.Join(mp.outDir, mp.dumpFileName)
		proxyCmd = fmt.Sprintf(
			`/sbin/minijail0 -e -- "%s" --set confdir="%s" -w "%s" &> "%s"`, mp.binaryPath, mp.confDir, mp.dumpFilePath, logFilePath)
	} else {
		proxyCmd = fmt.Sprintf(
			`/sbin/minijail0 -e -- "%s" --set confdir="%s" &> "%s"`, mp.binaryPath, mp.confDir, logFilePath)
	}
	cmd := testexec.CommandContext(ctx, "bash", "-c", proxyCmd)

	// Required for remote tast tests. mitmproxy is written in Python and uses the PyInstaller
	// to bundle the mitmproxy scripts and all its dependencies into a single file. The file
	// is written in a temp directory named "_MEI<random-number>" which is re-created for every
	// execution. The default location for the temp directory is the OS temp directory (/tmp on
	// Chrome OS). On Chrome OS, executables cannot be ran from the TMP file (b/40615995) so
	// local tast tests change the tmp dir to /usr/local/tmp; for remote tests, the temp dir
	// has to be manually changed.
	cmd.Env = append(cmd.Env, "TMPDIR=/usr/local/tmp")

	testing.ContextLogf(ctx, "mitmproxy: starting with cmd: %s", proxyCmd)
	if err := cmd.Start(); err != nil || waitForProcRunning(ctx) != nil {
		return errors.Wrap(err, "failed to start proxy server (terminated at startup?)")
	}

	if err = mp.configureNetwork(ctx, pidPath); err != nil {
		return errors.Wrap(err, "failed to configure the proxy server network namespace")
	}

	if mp.healthCheck {
		if err := mp.verifyProxyStart(ctx); err != nil {
			return errors.Wrap(err, "failed to health check proxy server")
		}
	}

	testing.ContextLog(ctx, "mitmproxy: started successfully")
	mp.cmd = cmd
	mp.isRunning = true
	return nil
}

// configureMitmdump creates the mitmdump configuration file from the existing mp.options and mp.scripts.
// The mitmdump config file format is yaml. Config file example:
//
// ---
// listen_port: 4040
// allowed_endpoints_yaml: /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/endpoints.yml
// scripts:
// - /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/allowed_endpoints.py
// - /usr/local/share/tast/data_pushed/go.chromium.org/tast-tests/cros/local/bundles/cros/meta/data/allowed_endpoints_yaml.py
func (mp *MitmProxy) configureMitmdump(ctx context.Context, path string) error {
	configs := []string{"---"}
	configs = append(configs, fmt.Sprintf(`listen_port: %d`, mp.port))
	configs = append(configs, mp.options...)

	if len(mp.scriptPaths) > 0 {
		scripts := append([]string{"scripts:"}, mp.scriptPaths...)
		configs = append(configs, strings.Join(scripts, "\n - "))
	}

	yamlConfig := strings.Join(configs, "\n")
	testing.ContextLog(ctx, "Using the mitmproxy configuration: ", yamlConfig)

	f, err := os.Create(path)

	if err != nil {
		return errors.Wrap(err, "failed to create config file")
	}
	defer f.Close()

	_, err = f.WriteString(yamlConfig)
	if err != nil {
		return errors.Wrap(err, "failed to write config file")
	}
	return nil
}

// configureNetwork calls patchpanel to setup the network namespace for the local proxy.
func (mp *MitmProxy) configureNetwork(ctx context.Context, pidPath string) error {
	pc, err := patchpanel.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create patchpanel client")
	}

	pid, err := mp.getPidFromPath(ctx, pidPath)
	if err != nil {
		return errors.Wrap(err, "failed to get mitmproxy process pid")
	}
	mp.pid = pid

	if err != nil {
		return errors.Wrap(err, "failed to fetch the mitmproxy processes")
	}

	fd, resp, err := pc.ConnectNamespace(ctx, int32(pid), "", true)
	if err != nil {
		return err
	}
	mp.lifelineFD = fd

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(resp.PeerIpv4Address))
	ip := net.IP(b)
	mp.host = ip.String()

	return nil
}

// getPidFromPath reads the pid of the proxy process from `pidPath`. The pid is required by
// patchpanel to setup an isolated network namespace for the proxy server with it's own network
// address.
func (mp *MitmProxy) getPidFromPath(ctx context.Context, pidPath string) (int, error) {
	var pid int
	// Wait for the proxy service to write the PID at `pidPath`.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		dat, err := os.ReadFile(pidPath)
		if err != nil {
			return errors.Wrap(err, "failed to read proxy process pid")
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(dat)))
		if err != nil {
			return errors.Wrap(err, "failed to parse process pid")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return 0, err
	}

	return pid, nil
}

// configurePidAddon creates a mitmproxy addon script which writes the pid
// of the proxy process to a file and returns the path to the file.
func (mp *MitmProxy) configurePidAddon() (string, error) {
	// Create a temp dir where configuration and pid files can be saved.
	pidFile, err := mp.createTempFile("mitmpid")
	if err != nil {
		return "", errors.Wrap(err, "failed to create PID file")
	}

	script := fmt.Sprintf(`import os

def running():
	with open("%s","w+") as f:
		f.write(str(os.getpid()))
`, pidFile)

	pidScript, err := mp.generateAddon("mitma_add_on", script)
	if err != nil {
		return "", errors.Wrap(err, "failed to create add on file")
	}

	mp.scriptPaths = append(mp.scriptPaths, pidScript)
	return pidFile, nil
}

// generateAddon creates addon file based on name and script.
func (mp *MitmProxy) generateAddon(name, script string) (string, error) {
	file, err := mp.createTempFile(fmt.Sprintf("%s*.py", name))
	if err != nil {
		return "", errors.Wrapf(err, "failed to create %s Addon", name)
	}

	err = os.WriteFile(file, []byte(script), 0755)
	if err != nil {
		return "", errors.Wrapf(err, "failed to write %s Addon", name)
	}
	return file, nil
}

// createTempFile creates a temporary file in proxy server's config directory and returns the
// path.
func (mp *MitmProxy) createTempFile(name string) (string, error) {
	file, err := os.CreateTemp(mp.confDir, name)
	if err != nil {
		return "", errors.Wrap(err, "failed create temp file")
	}
	defer file.Close()
	if err := os.Chmod(file.Name(), 0755); err != nil {
		return "", errors.Wrapf(err, "failed to chmod %v", file.Name())
	}
	return file.Name(), nil
}

// verifyProxyStart verifies that the proxy starts with a root certificate successfully
// by checking the magic domain served locally by mitmproxy.
// However, this check can be bypassed via `HealthCheck` option in case the domain won't work with a user's allowlist or blocklist.
func (mp *MitmProxy) verifyProxyStart(ctx context.Context) error {
	// Get cert.
	certFilePath, _, err := mp.RootCertificate(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find cert file path")
	}

	caCert, err := os.ReadFile(certFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read cert file")
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Setup http client.
	client, err := mp.httpClient(caCertPool)
	if err != nil {
		return nil
	}

	// This would pass if the magic domain returns a 200 OK without the following error message in response.
	const respRootCANotInstalled = "traffic is not passing through mitmproxy"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := client.Get(testenv.MagicURL)
		if err != nil {
			return errors.Wrap(err, "failed to visit the magic domain")
		}
		if resp.StatusCode != http.StatusOK {
			return errors.Errorf("%s returns %v, want %v", testenv.MagicURL, resp.StatusCode, http.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read response"))
		}
		if strings.Contains(string(body), respRootCANotInstalled) {
			return testing.PollBreak(errors.New("mitmproxy root certificate is not properly installed"))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 2 * time.Second}); err != nil {
		return errors.Wrap(err, "mitmproxy: failed to start with root certificate")
	}
	return nil
}

// RootCertificate returns the file path and the type (system or user) of the root certificate then ensures its existence.
func (mp *MitmProxy) RootCertificate(ctx context.Context) (string, string, error) {
	certType := UserCert
	if mp.customCA {
		certType = SystemCert
	}

	certFilePath := filepath.Join(mp.confDir, defaultKeyPairCertFile)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(certFilePath); err != nil {
			return errors.Wrapf(err, "%s is unavailable", certFilePath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return "", certType, errors.Wrapf(err, "failed to locate cert file at %q, maybe the proxy server is not launched successfully", certFilePath)
	}

	return certFilePath, certType, nil
}

// ProxyAddress returns the proxy address to be set in browser.
func (mp *MitmProxy) ProxyAddress() string {
	return fmt.Sprintf("%s:%d", mp.host, mp.port)
}

// Close closes proxy.
func (mp *MitmProxy) Close(ctx context.Context) error {
	if !mp.isRunning {
		testing.ContextLog(ctx, "mitmproxy: already closed, safe to ignore")
		return nil
	}

	var cleanupErrs []error

	// Disconnect from ash-chrome if it has been connected.
	if mp.disconnect != nil {
		if err := mp.disconnect(ctx); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to disconnect proxy from chrome"))
		}
		mp.disconnect = nil
	}

	// Terminate the proxy processes.
	if mp.cmd != nil {
		if err := killCmd(ctx, mp.cmd, mp.pid); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to stop proxy command"))
		}
		mp.cmd = nil
	}

	// Closing the fd will signal to patchpanel that it needs to tear down the network namespace
	// for the local proxy server.
	if mp.lifelineFD != nil {
		mp.lifelineFD.Close()
		mp.lifelineFD = nil
	}

	// Save a compressed full dump file.
	if mp.dumpFull {
		if _, err := os.Stat(mp.dumpFilePath); err == nil {
			targetTar := mp.dumpFilePath + ".tar.gz"
			if err := testexec.CommandContext(ctx, "tar", "-czf", targetTar, "-C", mp.outDir, mp.dumpFileName, "--remove-files").Run(testexec.DumpLogOnError); err != nil {
				cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to compress mitmproxy dump"))
			}
		}
	}

	if mp.removeCert {
		// Remove config files, such as certificates generated by Mitmproxy launch.
		if err := mp.removeCertDir(); err != nil {
			cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to clean mitmproxy config"))
		}
	}

	// If the system sees stale proxy connection that has already been terminated, it can lead to network issues for next tests.
	// Restart Chrome without proxy to prevent network issues caused by stale proxy connections.
	if ok, err := mp.IsConnected(ctx); err != nil {
		cleanupErrs = append(cleanupErrs, errors.Wrap(err, "failed to get proxy connection from the system"))
	} else if ok {
		testing.ContextLog(ctx, "mitmproxy: stale proxy connection found. proxy should be unset for a DUT to stay connected")
		if mp.forceUnsetOnClose {
			testing.ContextLog(ctx, "mitmproxy: restarting chrome to clear stale proxy connection")
			chrome.New(ctx)
		}
	}

	mp.isRunning = false
	if err := errors.Join(cleanupErrs...); err != nil {
		testing.ContextLog(ctx, "mitmproxy: closed with errors: ", err)
		return err
	}
	testing.ContextLog(ctx, "mitmproxy: closed successfully")
	return nil
}

// DumpHTTPFlow extracts HTTP flow information for analysis. To use this method,
// first enable the 'DumpHTTPFlow' option. Returns a map representing the HTTP flow
// on success, or an error if anything goes wrong.
//
// Parameters:
//   - reset (bool): If true, resets the HTTP flow data after extraction.
//   - saveToFile (bool): If true, saves the extracted flow information to a file
//     in 'outDir' in addition to returning the map.
//
// Returned DumpHTTPResponse pointer.
func (mp *MitmProxy) DumpHTTPFlow(ctx context.Context, reset, saveToFile bool) (*DumpHTTPResponse, error) {
	if !mp.dumpHTTPFlowEnabled {
		return nil, errors.New("DumpHTTPFlow is disabled, please enabled it by DumpHTTPFlow option")
	}

	// Create an HTTP client.
	client, err := mp.httpClient(nil)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("reset", strconv.FormatBool(reset))
	if saveToFile {
		params.Set("outDir", mp.outDir)
	}
	fullURL := testenv.DumpHTTPFlowURL + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new request")
	}
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	var result DumpHTTPResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrapf(err, "failed to convert to map, raw data: %s", string(body))
	}

	return &result, nil
}

func (mp *MitmProxy) httpClient(pool *x509.CertPool) (*http.Client, error) {
	proxyURLStr := "http://" + mp.ProxyAddress()
	proxyURL, err := url.Parse(proxyURLStr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse url: %s", proxyURLStr)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	if pool != nil {
		transport.TLSClientConfig = &tls.Config{
			RootCAs: pool,
		}
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

func (mp *MitmProxy) removeCertDir() error {
	if _, err := os.Stat(mp.confDir); err == nil {
		return os.RemoveAll(mp.confDir)
	}
	return nil
}

// Connect makes a new proxy connection to Chrome in-session.
// Once the proxy process is started, call this function to configure proxy for Chrome.
// Note that it would not work for Chrome in OOBE where `chrome.proxy.settings` is not functional. Pass chrome.ProxyServer to chrome.New instead.
func (mp *MitmProxy) Connect(ctx context.Context, cr *chrome.Chrome) error {
	// Log an error if this function is called unexpectedly (eg, from OOBE)
	hasOobePage := func() (bool, error) {
		return cr.IsTargetAvailable(ctx, func(t *target.Info) bool {
			return strings.HasPrefix(t.URL, "chrome://oobe") && t.Type == "page"
		})
	}
	if has, _ := hasOobePage(); has {
		return errors.New("ConnectTo should not be used for proxy in OOBE. Pass chrome.ProxyServer to chrome.New instead")
	}

	testing.ContextLog(ctx, "mitmproxy: connecting to chrome in-session")
	certPath, certType, err := mp.RootCertificate(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to locate CA certificate")
	}

	disconnect := func(ctx context.Context, cr *chrome.Chrome) error {
		var err error
		if err := clearChromeProxy(ctx, cr); err != nil {
			err = errors.Wrap(err, "failed to reset proxy in chrome")
		}
		// Clean up the user cert if it has been imported to user home.
		if certType == "user" {
			if err := removeRootCertFromNSS(ctx, cr); err != nil {
				err = errors.Wrap(err, "failed to remove root certificate in user home")
			}
		}
		if err != nil {
			testing.ContextLog(ctx, "mitmproxy: cleaned up chrome, err: ", err)
		} else {
			testing.ContextLog(ctx, "mitmproxy: cleaned up chrome successfully")
		}
		return err
	}
	mp.disconnect = func(ctx context.Context) error { return disconnect(ctx, cr) }

	// Copy a root CA certificate to user home if it is the user cert.
	if certType == "user" {
		if err := importRootCertToNSS(ctx, certPath, cr); err != nil {
			disconnect(ctx, cr)
			return errors.Wrap(err, "failed to import root certificate from proxy to chrome")
		}
	}
	// Configure an IP of the mitmproxy server in chrome
	if err := setChromeProxy(ctx, cr, mp.ProxyAddress()); err != nil {
		disconnect(ctx, cr)
		return errors.Wrap(err, "failed to configure proxy in chrome")
	}
	return nil
}
