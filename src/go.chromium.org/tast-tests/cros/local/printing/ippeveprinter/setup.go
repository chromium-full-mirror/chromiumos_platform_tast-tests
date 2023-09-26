// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ippeveprinter provides an interface to configure and run the
// `ippeveprinter` CUPS utility. It can be useful for tests that need a fake
// network printer with IPP capabilities.
// See https://www.cups.org/doc/man-ippeveprinter.html.
package ippeveprinter

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

// IppEverywherePrinter wraps the `ippeveprinter` CUPS utility.
type IppEverywherePrinter struct {
	// The running `ippeveprinter` process.
	*testexec.Cmd

	hostname string
	port     int
}

// config contains the information needed to run the `ippeveprinter` process.
type config struct {
	// The command-line arguments to `ippeveprinter`.
	args []string

	// This is the name of the printer as advertised using DNS-SD.
	// If DNS-SD is disabled, this will be set to 'ippeveprinter-name'.
	printerName string

	// Hostname set by the '-n' option.
	// This will be "localhost" if we're not passing '-n' to `ippeveprinter`.
	hostname string

	// Port set by the '-p' option.
	// This will be 0 if we're not passing '-p' to `ippeveprinter`.
	port int
}

// Option is an `ippeveprinter` command-line argument. Multiple options can be
// passed to Start().
type Option func(*config)

// WithAttributesFile sets the printer attributes file for `ippeveprinter` (-a option).
func WithAttributesFile(path string) Option {
	return func(c *config) {
		c.args = append(c.args, "-a", path)
	}
}

// WithKeyPath sets the location of server X.509 certificates and keys (-K option).
func WithKeyPath(path string) Option {
	return func(c *config) {
		c.args = append(c.args, "-K", path)
	}
}

// WithAuthenticationEnabled enables authentication (-A option).
func WithAuthenticationEnabled() Option {
	return func(c *config) {
		c.args = append(c.args, "-A")
	}
}

// WithDNSSD turns on DNS-SD service advertisements on `ippeveprinter`.
// `printerName` will be the advertised display name of the printer.
// This makes the printer auto-discoverable by mDNS clients and can interfere with
// other tests running on the same network.
func WithDNSSD(printerName string) Option {
	return func(c *config) {
		c.printerName = printerName
	}
}

// WithHostname sets the hostname that `ippeveprinter` listens on (-n option).
// The default is whatever the `hostname` command returns.
func WithHostname(hostname string) Option {
	return func(c *config) {
		c.args = append(c.args, "-n", hostname)
		c.hostname = hostname
	}
}

// WithPort sets the port that `ippeveprinter` listens on (-p option).
// If not given, `ippeveprinter` can choose any free port.
func WithPort(port int) Option {
	return func(c *config) {
		c.args = append(c.args, "-p", fmt.Sprint(port))
		c.port = port
	}
}

// WithVerbosityLevel corresponds to the -v[vvv] option.
// `level` has to be between 0 and 4, inclusive.
func WithVerbosityLevel(level int) Option {
	return func(c *config) {
		if level != 0 {
			c.args = append(c.args, fmt.Sprintf("-%s", strings.Repeat("v", level)))
		}
	}
}

// IppURI returns the ipp:// URI that should be used to communicate with the printer
// e.g. ipp://localhost:port/ipp/print.
func (pr *IppEverywherePrinter) IppURI() string {
	return fmt.Sprintf("ipp://%s:%d/ipp/print", pr.hostname, pr.port)
}

// IppsURI returns the ipps:// URI that should be used to communicate with the printer
// e.g. ipps://localhost:port/ipp/print.
func (pr *IppEverywherePrinter) IppsURI() string {
	return fmt.Sprintf("ipps://%s:%d/ipp/print", pr.hostname, pr.port)
}

// Port returns the local port that `ippeveprinter` is listening on.
func (pr *IppEverywherePrinter) Port() int {
	return pr.port
}

// Stop terminates the `ippeveprinter` process and logs errors if any.
func (pr *IppEverywherePrinter) Stop(ctx context.Context) error {
	return stopProcess(ctx, pr.Cmd)
}

// Start starts up `ippeveprinter` with the given options and waits until it's ready to accept requests.
func Start(ctx context.Context, opts ...Option) (*IppEverywherePrinter, error) {
	deviceHostname, err := os.Hostname()
	if err != nil {
		return nil, errors.Wrap(err, "could not get device hostname")
	}
	c := &config{
		args:        []string{"-r", "off"}, // We use '-r off' to disable DNS-SD by default.
		printerName: "ippeveprinter-name",
		hostname:    deviceHostname,
		port:        0,
	}

	for _, opt := range opts {
		opt(c)
	}

	// After options are processed, append `printerName` as ippeveprinter's last command-line argument.
	c.args = append(c.args, c.printerName)

	cmd, port, err := startPrinter(ctx, c)
	if err != nil {
		return nil, err
	}
	return &IppEverywherePrinter{
		Cmd:      cmd,
		hostname: c.hostname,
		port:     port,
	}, nil
}

func waitUntilListening(ctx context.Context, host string, port int, timeout time.Duration) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		_, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		return err
	}, &testing.PollOptions{Timeout: timeout})
}

func startPrinter(ctx context.Context, c *config) (*testexec.Cmd, int, error) {
	// We use `stdbuf` on top of `ippeveprinter` to prevent output bufferring.
	stdbufOpts := []string{"-o0", "ippeveprinter"}
	stdbufOpts = append(stdbufOpts, c.args...) // ippeveprinter's options.

	ippeveprinter := testexec.CommandContext(ctx, "stdbuf", stdbufOpts...)
	stderr, err := ippeveprinter.StderrPipe()
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to fetch stderr")
	}

	if err := ippeveprinter.Start(); err != nil {
		ippeveprinter.DumpLog(ctx)
		return nil, 0, errors.Wrap(err, "failed to start ippeveprinter")
	}

	var printerPort int

	// If port is given, `ippeveprinter` does not print it, so we just use what
	// was given.
	if c.port != 0 {
		printerPort = c.port
	} else {
		printerPort, err = parsePort(ctx, bufio.NewReader(stderr))
		if err != nil {
			stopProcess(ctx, ippeveprinter)
			return nil, 0, errors.Wrap(err, "failed to parse printer port")
		}
	}

	// Wait until printer started accepting requests.
	// It starts very quickly so 5 seconds is plenty of time to wait.
	if err := waitUntilListening(ctx, c.hostname, printerPort, time.Second*5); err != nil {
		return nil, printerPort, errors.Wrap(err, "timed out waiting for ippeveprinter to start")
	}

	return ippeveprinter, printerPort, nil
}

func parsePort(ctx context.Context, rd *bufio.Reader) (int, error) {
	r := regexp.MustCompile(`Listening on port (\d+)\.`)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return 0, errors.Wrap(err, "failed to read output from pipe")
		}
		matches := r.FindStringSubmatch(line)
		if matches != nil {
			port, err := strconv.Atoi(matches[1])
			if err != nil {
				return 0, errors.Wrap(err, "failed to parse printer port")
			}
			return port, nil
		}
	}
}

func stopProcess(ctx context.Context, cmd *testexec.Cmd) error {
	testing.ContextLogf(ctx, "Terminating process with PID %d", cmd.Cmd.Process.Pid)
	if err := cmd.Signal(unix.SIGTERM); err != nil {
		return errors.Wrap(err, "failed to send SIGTERM to process")
	}
	if err := cmd.Wait(); err != nil {
		// We are expecting the process to exit due to the SIGTERM signal we sent to it.
		// If the program has a signal handler, it can return any error code it wants,
		// but 143 is the canonical error code for SIGTERM so it's okay to rely on it in
		// most cases.
		const expectedExitCode = 143
		exitCode, ok := testexec.ExitCode(err)
		if !ok {
			cmd.DumpLog(ctx)
			return errors.Wrap(err, "failed to get process exit status")

		}
		if exitCode != expectedExitCode {
			cmd.DumpLog(ctx)
			return errors.Wrapf(err, "expected exit code %d, got %d", expectedExitCode, exitCode)
		}
	}
	return nil
}
