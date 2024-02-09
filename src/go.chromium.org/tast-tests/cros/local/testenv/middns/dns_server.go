// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package middns implements the middle layer DNS server using dnsmasq to redirect client requests for e2e testing.
package middns

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/testenv/provision"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	dnsServerBin = "/usr/sbin/dnsmasq"
	dnsPort      = 53
)

// DNSServer represents a middle-layer DNS server running on a DUT.
type DNSServer struct {
	IP           string
	pidFileName  string
	confFileName string
	lifelineFD   *os.File
	tempDir      string
	shillService *shill.Service
	HasStarted   bool
}

// FromTo is a pair of (from, to) of hostname
type FromTo struct {
	From string
	To   string
}

// NewDNSServer creates a new DNSServer.
// Note that the following conditions should be met for it to work:
// - Default network is not changed when DNSServer is running.
// - 8.8.8.8 is reachable
func NewDNSServer() *DNSServer {
	return &DNSServer{}
}

// Start starts a DNS server.
// Callers should call Stop to ensure all resources are cleaned up. Otherwise, it may cause other subsequent tests using network to fail.
//
// Example of `hostmap`,
//
// To redirect all subdomains of example.com to 1.1.1.1,
//
//	{From: "example.com", To: "1.1.1.1"}
//
// To redirect all subdomains of example.com to 1.1.1.1 except for certain subdomains to 2.2.2.2
//
//	{From: "example.com", To: "1.1.1.1"},
//	{From: "except.example.com", To: "2.2.2.2"}
//
// To bypass the DNS server and send query directly to the upstream nameservers, use "#" as a wildcard
//
//	{From: "www.example.com", To: "#"},
func (s *DNSServer) Start(ctx context.Context, hostmap []FromTo) (retErr error) {
	if s.HasStarted {
		return errors.New("DNS server already running, skipping Start")
	}

	// Create a PID file in a temp dir. "middns" stands for the middle-layer DNS.
	tempDir, err := os.MkdirTemp("", "middns-")
	if err != nil {
		return errors.Wrap(err, "failed to create temp dir")
	}
	defer func() {
		if _, err := os.Stat(tempDir); err == nil && retErr != nil {
			os.RemoveAll(tempDir)
		}
	}()
	if err := os.Chmod(tempDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to chmod %v", tempDir)
	}
	s.tempDir = tempDir
	pidFile, err := TempFile(s.tempDir, "middns-*.pid")
	if err != nil {
		return errors.Wrap(err, "failed to create PID file")
	}
	defer pidFile.Close()
	s.pidFileName = pidFile.Name()

	// Start the DNS server using dnsmasq with the configuration
	s.HasStarted = true
	provision.Marker.Acquire()
	defer func() {
		if retErr != nil {
			s.Stop(ctx)
		}
	}()

	// Configure dnsmasq and run with the configuration
	if err := s.configureDnsmasq(ctx, hostmap); err != nil {
		return errors.Wrap(err, "failed to configure dnsmasq")
	}
	cmd := testexec.CommandContext(ctx,
		"/sbin/minijail0", "-e", "--", dnsServerBin, "--conf-file="+s.confFileName)
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to start DNS server with cmd: %v", cmd)
	}

	// Connect DNS server isolated network namespace with upstream network
	if err := s.configureNetwork(ctx); err != nil {
		return errors.Wrap(err, "failed to setup the network namespace")
	}
	testing.ContextLogf(ctx, "DNS server started at %v with cmd: %v", s.IP, cmd)

	// Set shill to propagate the IPv4 of the DNS server.
	manager, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create shill manager")
	}
	service, err := manager.GetDefaultService(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find default service")
	}
	s.shillService = service
	if err := s.setStaticIPNameServers(ctx, []string{s.IP}); err != nil {
		return errors.Wrap(err, "failed to set the DNS server")
	}
	testing.ContextLog(ctx, "DNS server static IP address set to: ", s.shillService)
	return nil
}

// Stop stops the DNS server.
func (s *DNSServer) Stop(ctx context.Context) error {
	if !s.HasStarted {
		return nil
	}

	defer func() {
		if s.tempDir != "" {
			os.RemoveAll(s.tempDir)
		}
		s.HasStarted = false
	}()

	var err error
	// Clear the static IP of the DNS server that has been set.
	if s.shillService != nil {
		if err = s.setStaticIPNameServers(ctx, []string{}); err != nil {
			err = errors.Wrap(err, "failed to reset the DNS IP after use")
		}
	}
	// Closing the fd will signal to patchpanel that it needs to tear down the network namespace
	// for the local server.
	if s.lifelineFD != nil {
		s.lifelineFD.Close()
	}

	// Kill the current dnsmasq process
	if pid, err := PidFromPath(ctx, s.pidFileName); err != nil {
		err = errors.Wrap(err, "failed to get proxy pid")
	} else if err := testexec.CommandContext(ctx, "kill", fmt.Sprintf("%d", pid)).Run(); err != nil {
		err = errors.Wrap(err, "failed to kill dnsmasq process")
	}

	if err != nil {
		// If there is an unrecoverable error even after reboot,
		// don't remove the marker file so the infra can attempt to repair a DUT.
		testing.ContextLog(ctx, "DNS server stopped with error: ", err)
		return err
	}
	provision.Marker.Release()
	testing.ContextLog(ctx, "DNS server stopped successfully")
	return nil
}

// setStaticIPNameServers sets shill to point to the static IP of the DNS server on
// the default network service.
// Callers should reset the setting after use by calling this func with an empty string address.
func (s *DNSServer) setStaticIPNameServers(ctx context.Context, ipaddrs []string) error {
	// Set shill to propagate the IPv4 of the DNS server.
	// For the simplicity in testing, we accept only one IP address for local DNS server.
	staticIPProps := map[string]interface{}{
		shillconst.IPConfigPropertyNameServers: ipaddrs,
	}
	if err := s.shillService.SetProperty(ctx, shillconst.ServicePropertyStaticIPConfig, staticIPProps); err != nil {
		return errors.Wrap(err, "failed to set property")
	}

	// Verify that it overrides the IPs of the DNS server in shill.
	actualIPAddrs, err := s.getStaticIPNameServers(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get static IP of name servers")
	}
	if !slices.Equal(actualIPAddrs, ipaddrs) {
		return errors.Wrapf(err, "failed to find the name servers in shill, actual: %v, expected: %v", actualIPAddrs, ipaddrs)
	}
	return nil
}

func (s *DNSServer) getStaticIPNameServers(ctx context.Context) ([]string, error) {
	props, err := s.shillService.GetProperties(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get network service property")
	}
	actualIPProps, err := props.Get(shillconst.ServicePropertyStaticIPConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get static IP config")
	}
	if actualIPAddrs, ok := actualIPProps.(map[string]interface{})[shillconst.IPConfigPropertyNameServers]; ok {
		return actualIPAddrs.([]string), nil
	}
	return []string{}, nil
}

// configureDnsmasq configures the local DNS server with upstream nameservers specified and
// returns command line args used to start the DNS server.
func (s *DNSServer) configureDnsmasq(ctx context.Context, hostmap []FromTo) error {
	fr, err := TempFile(s.tempDir, "resolv-*.conf")
	if err != nil {
		return errors.Wrap(err, "failed to create new resolv.conf")
	}
	defer fr.Close()

	// Skip reading the default DNS proxy (/etc/resolv.conf) when dnsmasq is started.
	// It prevents DNS query from dnsmasq to loop back to DNS proxy.
	// Instead of /etc/resolv.conf, dnsmasq reads a new file that points to 8.8.8.8 in case
	// the DNS server could not resolve hosts in the given hostmap.
	// TODO(b/323427320): Consider using the current nameserver instead of the hardcoded 8.8.8.8.
	const c = `nameserver 8.8.8.8`
	if _, err := fr.WriteString(c); err != nil {
		return errors.Wrap(err, "failed to write resolv.conf")
	}

	var args []string
	args = append(args, fmt.Sprintf(`resolv-file=%s`, fr.Name()))
	args = append(args, fmt.Sprintf(`pid-file=%s`, s.pidFileName))

	// Set a host redirection map with --address (and --server for bypass)
	// More specific domains take precedence over less specific domains.
	// Example,
	// --address=/example.com/1.2.3.4 --address=/www.example.com/2.3.4.5 will send queries for example.com and a.example.com to 1.2.3.4, but www.example.com will go to 2.3.4.5
	// --address=/example.com/1.2.3.4 --server=/www.example.com/# will send queries for example.com and its subdomains to 1.2.3.4, except www.example.com (and its subdomains) which will be forwarded as usual
	for _, h := range hostmap {
		from := h.From
		to := h.To
		// TODO(b/323427320): Warn if /etc/hosts overrides the same host with a different IP already that would cause this query to be ignored.
		// If "#" is a destination (bypass redirection), send queries to the upstream nameservers
		if to == "#" {
			args = append(args, fmt.Sprintf(`server=/%s/%s`, from, "#"))
			continue
		}
		addrs, err := net.LookupHost(to)
		if err != nil {
			return errors.Wrap(err, "failed to look up host")
		}
		// Sort and return only the first valid address for override.
		sort.Slice(addrs, func(i, j int) bool {
			return bytes.Compare(net.ParseIP(addrs[i]), net.ParseIP(addrs[j])) < 0
		})
		var valid string
		for _, addr := range addrs {
			if net.ParseIP(addr) != nil {
				valid = addr
				break
			}
		}
		if valid == "" {
			return errors.Errorf("no valid address found for %v", to)
		}
		args = append(args, fmt.Sprintf(`address=/%s/%s`, from, addrs[0]))
	}

	// Write the args to a config file.
	fd, err := TempFile(s.tempDir, "dnsmasq-*.conf")
	if err != nil {
		return errors.Wrap(err, "failed to create new dnsmasq.conf")
	}
	defer fd.Close()
	for _, arg := range args {
		if _, err := fd.WriteString(arg + "\n"); err != nil {
			return errors.Wrap(err, "failed to write dnsmasq.conf")
		}
	}
	s.confFileName = fd.Name()
	return nil
}

// configureNetwork calls patchpanel to setup the network namespace for the local DNS server.
func (s *DNSServer) configureNetwork(ctx context.Context) error {
	pc, err := patchpanel.New(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create patchpanel client")
	}

	pid, err := PidFromPath(ctx, s.pidFileName)
	if err != nil {
		return errors.Wrap(err, "failed to get proxy pid")
	}

	fd, resp, err := pc.ConnectNamespace(ctx, int32(pid),
		"", /* outbound_physical_device: empty to track the default network. */
		true /* allow_user_traffic: true to allow the traffic to be reachable to and from Chrome, ARC, etc */)
	if err != nil {
		return err
	}
	s.lifelineFD = fd

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(resp.PeerIpv4Address))
	ip := net.IP(b)
	if ip == nil {
		return errors.New("failed to parse DNS IP")
	}
	s.IP = ip.String()
	return nil
}

// PidFromPath returns the pid of the process from `pidPath`
func PidFromPath(ctx context.Context, pidPath string) (int, error) {
	var pid int
	// Wait for the process to write the PID at `pidPath`.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		dat, err := os.ReadFile(pidPath)
		if err != nil {
			return errors.Wrap(err, "failed to read process pid")
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(dat)))
		if err != nil {
			return errors.Wrap(err, "failed to parse process pid (already aborted?)")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return 0, err
	}
	return pid, nil
}

// TempFile creates a temp file for the DNS server with 0644 perm by default. Callers should close the file after use.
func TempFile(dir, pattern string) (f *os.File, err error) {
	f, err = os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create temp file, dir: %v, pattern: %v", dir, pattern)
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	if err := os.Chmod(f.Name(), 0644); err != nil {
		return nil, errors.Wrapf(err, "failed to chmod %v", f.Name())
	}
	return f, nil
}
