// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dhclient"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/wpasupplicant"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ShillTethering,
		Desc:     "Verify the behavior of the tethering feature",
		Contacts: []string{"cros-networking@google.com", "akahuang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      10 * time.Minute,
	})
}

const (
	// The fake user profile for testing.
	profileName = "test"

	// The SSID and passphrase of the hotspot WiFi.
	ssid       = "SSID"
	passphrase = "PASSWORD"

	// The hostname of the downstream client.
	hostname = "ClientHostName"

	// The response of HTTP server on the upstream.
	httpResp = "pong"

	// The absolute path of each linux command.
	curlCmdPath     = "/usr/bin/curl"
	ifconfigCmdPath = "/bin/ifconfig"
	ipCmdPath       = "/bin/ip"
	iwCmdPath       = "/usr/sbin/iw"
)

// ShillTethering verifies the behavior of the tethering feature.
// To simulate the tethering environment locally, the test uses mac80211_hwsim
// to create simulated WiFi interfaces, and move one of the hwsim interface into
// netns as a downstream device. Also, it creates another netns as the upstream
// router and http server. The network topology is drawn below.
//
// -                                    +-----------------------+
// -  +------------------------------+  | interacted with shill |  +------------------------------+
// -  |netns-ds (downstream)         |  +---+-------------+-----+  |netns-router (upstream)       |
// -  |                              |      |             |        |                              |
// -  |  wpa_supplicant              |      v             v        |                   dnsmasq    |
// -  |        dhclient --> wlan2 <--+--> wlan1     etho_router <--+-> ethi_router <-- httpserver |
// -  |                       ^      |      ^                      |                              |
// -  +-----------------------+------+      |                      +------------------------------+
// -                          |             |
// -                       +--+-------------+----+
// -                       |      created by     |
// -                       |    mac80211_hwsim   |
// -                       +---------------------+
//
// After the network topology is established, the function verifies:
// - The downstream could get the IPv4 DHCP IP
// - The downstream could send HTTP request to the upstream
// - The patchpanel could get the client's hostname
func ShillTethering(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Get the WiFi interface names created by the hwsim.
	wifiIfaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	apIface := wifiIfaces.AP[0]
	clientIface := wifiIfaces.AP[1]
	apPhyIndex, err := getWiPhyIndex(ctx, apIface)
	if err != nil {
		s.Fatal("Failed to get phy of ", apIface, ": ", err)
	}

	// Connect to shill's Manager and use the fake user profile.
	shillMgr, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}

	shillMgr.RemoveFakeUserProfile(ctx, profileName) // We don't care the failure of this function.
	if _, err := shillMgr.CreateFakeUserProfile(ctx, profileName); err != nil {
		s.Fatal("Failed to create fake profile: ", err)
	}
	defer shillMgr.RemoveFakeUserProfile(cleanupCtx, profileName)

	// Setup the virtual ethernet upstream.
	pool := subnet.NewPool()
	opt := virtualnet.EnvOptions{
		EnableDHCP: true,
		RAServer:   true,
		HTTPServerResponseHandler: func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, httpResp)
		},
	}
	_, router, err := virtualnet.CreateRouterEnv(ctx, shillMgr, pool, opt)
	if err != nil {
		s.Fatal("Failed to create router env: ", err)
	}
	defer router.Cleanup(cleanupCtx)

	// Configure and enable the tethering, and wait until the tethering is enabled.
	s.Log("Enable the tething with the interface ", apIface, " with phy_index ", apPhyIndex, " into shill")
	if err := shillMgr.SetTetheringAllowed(ctx, true); err != nil {
		s.Fatal("Failed to enable tethering: ", err)
	}
	defer shillMgr.SetTetheringAllowed(cleanupCtx, false)

	serviceProps := map[string]interface{}{
		shillconst.TetheringConfBand:                      "2.4GHz",
		shillconst.TetheringConfSSID:                      hex.EncodeToString([]byte(ssid)),
		shillconst.TetheringConfPassphrase:                passphrase,
		shillconst.TetheringConfUpstreamTech:              shillconst.TypeEthernet,
		shillconst.TetheringConfAutoDisable:               false,
		shillconst.TetheringConfDownstreamDeviceForTest:   apIface,
		shillconst.TetheringConfDownstreamPhyIndexForTest: apPhyIndex,
	}
	if err := shillMgr.ConfigureTethering(ctx, serviceProps); err != nil {
		s.Fatal("Failed to configure tethering: ", err)
	}

	if err := shillMgr.EnableTethering(ctx); err != nil {
		s.Fatal("Failed to enable tethering: ", err)
	}
	defer shillMgr.DisableTethering(cleanupCtx)

	if _, err := shillMgr.WaitForTetheringState(ctx, shillconst.TetheringStateActive); err != nil {
		s.Fatal("Failed to wait for active tethering state: ", err)
	}

	// Setup the netns for virtual WiFi downstream, and move the interface into it.
	dsEnv, err := virtualnet.CreateEnv(ctx, "ds")
	if err != nil {
		s.Fatal("Failed to create downstream env: ", err)
	}
	defer dsEnv.Cleanup(cleanupCtx)

	s.Log("Move the interface ", clientIface, " into downstream netns")
	phy, err := getWiPhyIndex(ctx, clientIface)
	if err != nil {
		s.Fatal("Failed to get Wiphy of hwsim interface: ", err)
	}
	if err := testexec.CommandContext(
		ctx, iwCmdPath, "phy", "phy"+strconv.FormatUint(uint64(phy), 10),
		"set", "netns",
		"name", "/run/netns/"+dsEnv.NetNSName).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to move the interface into netns: ", err)
	}
	if err := dsEnv.CreateCommandWithoutChroot(
		ctx, ifconfigCmdPath, clientIface, "up").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to bring up the interface at netns: ", err)
	}

	// Run wpa_supplicant and dhclient on the downstream interface.
	supplicant := wpasupplicant.New(clientIface, ssid, passphrase)
	if err := dsEnv.StartServer(ctx, "wpa_supplicant", supplicant); err != nil {
		s.Fatal("Failed to start wpa_supplicant in downstream env: ", err)
	}
	dhclient := dhclient.New(clientIface, hostname)
	if err := dsEnv.StartServer(ctx, "dhclient", dhclient); err != nil {
		s.Fatal("Failed to start dhclient in downstream env: ", err)
	}

	// Verify whether the downstream could get the HTTP response from upstream.
	// TODO(b/288359516): Verify the IPv6 HTTP request as well.
	addrs, err := router.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get router's IP: ", err)
	}
	if resp, err := dsEnv.CreateCommandWithoutChroot(ctx, curlCmdPath, addrs.IPv4Addr.String()).CombinedOutput(); err != nil {
		s.Fatal("Failed to get HTTP response via IPv4: ", err)
	} else if strings.TrimSpace(string(resp[:])) != httpResp {
		s.Fatal("Got wrong HTTP response content: ", resp)
	}
	s.Log("Got the correct HTTP response via IPv4")

	// Verify whether the patchpanel gets the correct hostname.
	pc, err := patchpanel.New(ctx)
	if err != nil {
		s.Fatal("Failed to create patchpanel client: ", err)
	}
	response, err := pc.GetDownstreamNetworkInfo(ctx, apIface)
	if err != nil {
		s.Fatal("Failed to call patchpanel.GetDownstreamNetworkInfo(): ", err)
	}
	if len(response.GetClientsInfo()) == 0 {
		s.Fatal("client info is empty")
	}
	if response.GetClientsInfo()[0].GetHostname() != hostname {
		s.Fatalf("The hostname `%s` is wrong, the expect hostname is `%s`",
			response.GetClientsInfo()[0].GetHostname(), hostname)
	}
	s.Log("The client's hostname is verified")

	// TODO(b/288359516): Verify if the downstream device could get the custom MTU from upstream.
	// TODO(b/273749806): Verify if the upstream device could not reach the downstream device.
}

func getWiPhyIndex(ctx context.Context, iface string) (uint32, error) {
	cmd := fmt.Sprintf(`%s %s info | grep wiphy | awk '{print $2}'`, iwCmdPath, iface)
	out, err := testexec.CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get iw info")
	}

	phyIdx, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "failed to convert to integer")
	}
	return uint32(phyIdx), nil
}
