// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wireless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/wificell/router/openwrt/uci"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/timing"
)

// Server controls a uci on router.
type Server struct {
	host       *ssh.Conn
	name       string
	config     *Config
	workDir    string
	uci        *uci.Runner
	stdoutFile *os.File
	stderrFile *os.File
}

// StartServer creates a new Server object of wireless configuration the given host.
// workDir is the dir on host for the server to put temporary files.
// name is the identifier used for log filenames in OutDir.
// After getting a Server instance, s, the caller should call s.Close() at the end, and use the
// shortened ctx (provided by s.ReserveForClose()) before s.Close() to reserve time for it to run.
func StartServer(ctx context.Context, host *ssh.Conn, uci *uci.Runner, name string, config *Config) (server *Server, retErr error) {
	s := &Server{
		host:   host,
		uci:    uci,
		name:   name,
		config: config,
	}
	// Clean up on error.
	defer func() {
		if retErr != nil {
			// Close the Server instance created above, not the returned one as it might be nil.
			s.Close(ctx)
		}
	}()
	if err := s.start(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) start(fullCtx context.Context) (retErr error) {
	fullCtx, st := timing.Start(fullCtx, "start")
	defer st.End()

	defer func() {
		if retErr != nil {
			s.Close(fullCtx)
		}
	}()
	ctx, cancel := s.ReserveForClose(fullCtx)
	defer cancel()
	if err := s.initConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to init wireless config")
	}
	if err := s.host.CommandContext(ctx, "sync").Run(); err != nil {
		return errors.Wrap(err, "failed to sync wireless config")
	}
	if err := uci.RestartWifi(ctx, s.uci); err != nil {
		return errors.Wrap(err, "failed to restart WiFi")
	}
	return nil
}

// Close stops uci and cleans up related resources.
func (s *Server) Close(ctx context.Context) error {
	return uci.StopWifi(ctx, s.uci)
}

// initConfig writes hostapd config files of all ifaces.
func (s *Server) initConfig(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "initConfig")
	defer st.End()
	quietFlag := uci.CLIFlagQuietMode()

	// Init wifi-device configs
	for _, device := range s.config.DeviceConfigs {
		s.uci.Set(ctx, uci.ConfigWireless, device.Name, "", WiFiDevice, quietFlag)
		s.uci.Set(ctx, uci.ConfigWireless, device.Name, "type", device.Type, quietFlag)
		s.uci.Set(ctx, uci.ConfigWireless, device.Name, "channel", strconv.Itoa(device.Channel), quietFlag)
		if device.Disabled {
			s.uci.Set(ctx, uci.ConfigWireless, device.Name, "disabled", "1", quietFlag)
		}
		s.uci.Set(ctx, uci.ConfigWireless, device.Name, "band", device.Band, quietFlag)
		s.uci.Set(ctx, uci.ConfigWireless, device.Name, "htmode", string(device.HtMode), quietFlag)
		if device.Country != "" {
			s.uci.Set(ctx, uci.ConfigWireless, device.Name, "country", device.Country, quietFlag)
		}
		if device.HtExtcha {
			s.uci.Set(ctx, uci.ConfigWireless, device.Name, "ht_extcha", "1", quietFlag)
		}
	}

	// Init wifi-iface configs
	for _, iface := range s.config.IfaceConfigs {
		s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "", WiFiIface, quietFlag)
		if iface.VifIdx > 0 {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "vifidx", strconv.Itoa(iface.VifIdx), quietFlag)
		}
		s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "device", iface.Device, quietFlag)
		s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "network", iface.Network, quietFlag)
		if iface.Disabled {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "disabled", "1", quietFlag)
		}
		s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "mode", iface.Mode, quietFlag)
		if iface.SSID != "" {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "ssid", iface.SSID, quietFlag)
		}
		if iface.BSSID != "" {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "bssid", iface.BSSID, quietFlag)
		}
		if iface.Encryption != "" {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "encryption", iface.Encryption, quietFlag)
		}
		if len(iface.Pairwise) != 0 {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "pairwise", strings.Join(iface.Pairwise, ","), quietFlag)
		}
		if iface.GroupCipher != "" {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "group_cipher", iface.GroupCipher, quietFlag)
		}
		if iface.GroupMgmtCipher != "" {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "group_mgmt_cipher", iface.GroupMgmtCipher, quietFlag)
		}
		if iface.Key != "" {
			s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "key", iface.Key, quietFlag)
		}
		s.uci.Set(ctx, uci.ConfigWireless, iface.Name, "ieee80211w", strconv.Itoa(iface.Ieee80211w), quietFlag)
	}

	// Init wifi-mld configs
	for _, mld := range s.config.MldConfigs {
		s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "", WiFiMld, quietFlag)
		if mld.Disabled {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "disabled", "1", quietFlag)
		}
		s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "mode", mld.Mode, quietFlag)
		if len(mld.Ifaces) != 0 {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "iface", strings.Join(mld.Ifaces, " "), quietFlag)
		}
		if mld.SSID != "" {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "ssid", mld.SSID, quietFlag)
		}
		if mld.Encryption != "" {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "encryption", mld.Encryption, quietFlag)
		}
		if mld.Key != "" {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "key", mld.Key, quietFlag)
		}
		s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "ieee80211w", strconv.Itoa(mld.Ieee80211w), quietFlag)
		if mld.PmfSha256 {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "pmf_sha256", "1", quietFlag)
		}
		if mld.SaePassword != "" {
			s.uci.Set(ctx, uci.ConfigWireless, mld.Name, "sae_password", mld.SaePassword, quietFlag)
		}
	}

	return uci.CommitConfig(ctx, s.uci, uci.ConfigWireless)
}

// ReserveForClose returns a shortened ctx with cancel function.
// The shortened ctx is used for running things before s.Close() to reserve time for it to run.
func (s *Server) ReserveForClose(ctx context.Context) (context.Context, context.CancelFunc) {
	return ctxutil.Shorten(ctx, 2*time.Second)
}

// Config returns a copy of the config of the server.
func (s *Server) Config() *Config {
	config := *s.config
	return &config
}

// Name returns the name of the server.
func (s *Server) Name() string {
	return s.name
}
