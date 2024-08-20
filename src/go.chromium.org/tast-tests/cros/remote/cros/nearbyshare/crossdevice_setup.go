// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dhcp"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "crossDeviceRemote",
		Desc: "Fixture for setting up environment for Cross Device local tests",
		Contacts: []string{"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com"},
		BugComponent: "b:1108889", // ChromeOS > Software > System Services > Cross Device
		Impl:         &nearbyRemoteFixture{},
		ServiceDeps: []string{
			wificell.ShillServiceName,
		},
		Parent:          wificell.FixtureID(wificell.TFFeaturesCapture),
		SetUpTimeout:    3 * time.Minute,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: 4 * time.Minute,
		PreTestTimeout:  resetTimeout,
		PostTestTimeout: resetTimeout,
	})
}

type nearbyRemoteFixture struct {
	labstationConn *ssh.Conn
	serialNumber   string
	wf             *wificell.TestFixture
	apIface        *wificell.APIface
	ssid           string
}

func (f *nearbyRemoteFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Enable wifi on CrOS.
	dut1 := s.DUT()
	r, err := rpc.Dial(ctx, dut1, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect rpc: ", err)
	}
	defer r.Close(ctx)

	wifiClient := wifi.NewShillServiceClient(r.Conn)
	if _, err := wifiClient.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{Enabled: true}); err != nil {
		s.Error("Could not enable Wifi through shill: ", err)
	}

	// Set up WiFi AP.
	wf := s.ParentValue().(*wificell.TestFixture)
	f.wf = wf
	apOpts := []hostapd.Option{hostapd.Mode(hostapd.Mode80211a), hostapd.Channel(48)}
	passphrase := "password"
	fac := wpa.NewConfigFactory(
		passphrase,
		wpa.Mode(wpa.ModePureWPA2),
		wpa.Ciphers2(wpa.CipherCCMP),
	)
	dnsOpt := new(dhcp.DNSOption)
	dnsOpt.Port = 53
	dnsOpt.NameServers = []string{"8.8.8.8", "8.8.4.4"}
	// Setting no-op value value, because if left empty,
	// all domains are resolved to a specific IP
	dnsOpt.ResolvedHost = "nohost"
	apIface, err := wf.ConfigureAPOnRouterIDWithConfsDNSOpts(ctx, 0, []hostapd.ApConfig{{ApOpts: apOpts, SecConfFac: fac}}, "", true, true, false, dnsOpt)
	// apIface, err := wf.ConfigureAPOnRouterID(ctx, 0, apOpts, fac, true, false)
	if err != nil {
		s.Fatal("Failed to configure ap, err: ", err)
	}
	f.apIface = apIface
	f.ssid = apIface.Config().SSID
	s.Log("Wificell AP SSID: ", f.ssid)

	// Run script to allow internet connection on the AP.
	enableInternetScript := "iptables -I FORWARD 1 -i managed1 -o eth0 -j ACCEPT;" +
		"iptables -I FORWARD 2 -i eth0 -o managed1 -m state --state ESTABLISHED,RELATED -j ACCEPT;" +
		"iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE;" +
		"iptables -A INPUT -i managed1 -j ACCEPT;" +
		"iptables -A INPUT -i eth0 -m state --state ESTABLISHED,RELATED -j ACCEPT;" +
		"iptables -A OUTPUT -j ACCEPT;" +
		"echo 1 > /proc/sys/net/ipv4/ip_forward"
	apConn := wf.APConn()
	if err := apConn.CommandContext(ctx, "sh", "-c", enableInternetScript).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to run iptables commands to enable Internet access: ", err)
	}
	s.Log("Internet access enabled on AP")
	s.Log("AP setup done")

	_, err = wf.ConnectWifiAP(ctx, apIface)
	if err != nil {
		s.Fatal("Failed to connect to WiFi, err: ", err)
	}
	s.Log("Connected")

	// Get Android companion DUT info
	companions, err := android.Companions()
	if err != nil {
		s.Log("Failed to find Android companion devices, falling back to local USB run: ", err)
		return ""
	}
	for i, c := range companions {
		testing.ContextLogf(ctx, "Android companion %d: associate %q serial %q model %q",
			i, c.AssociatedHostname, c.SerialNumber, c.ModelName)
	}

	// Connect to the labstation. Use ProxyCommand so you don't have to port forward.
	sshOptions := &ssh.Options{
		KeyDir:       s.DUT().KeyDir(),
		KeyFile:      s.DUT().KeyFile(),
		ProxyCommand: s.DUT().ProxyCommand(),
	}
	if err := ssh.ParseTarget(companions[0].AssociatedHostname, sshOptions); err != nil {
		s.Fatal("Failed to parse labstation ssh target: ", err)
	}
	d2, err := ssh.New(ctx, sshOptions)
	if err != nil {
		s.Fatal("Failed to connect to labstation phone host over ssh: ", err)
	}

	// Setup the phone for adb-over-wifi
	// Restart adb server
	if err := d2.CommandContext(ctx, "adb", "kill-server").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to kill any running adb server: ", err)
	}
	if err := d2.CommandContext(ctx, "mkdir", "-p", "/run/arc/adb").Run(); err != nil {
		s.Fatal("Failed to make arc dir")
	}

	// Required to start adb server with the ARC++ adb key for touchless setup.
	// /var/lib/android_keys is a fixed location on the scheduling unit's labtation that is configured by the lab to include the right keys.
	cmdStrs := []string{
		"ADB_VENDOR_KEYS=/var/lib/android_keys",
		"adb start-server",
	}
	if err := d2.CommandContext(ctx, "sh", "-c", strings.Join(cmdStrs, " ")).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to start adb with correct permissions: ", err)
	}

	// Print the existing devices attached to the labstation.
	out, err := d2.CommandContext(ctx, "adb", "devices").Output()
	if err != nil {
		s.Fatal("Failed to run adb devices")
	}
	s.Log(string(out))

	// Restart adb as root
	if err := d2.CommandContext(ctx, "adb", "-s", companions[0].SerialNumber, "root").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to restart adb as root: ", err)
	}

	// Connect the Android device to the WiFi AP.
	if err := d2.CommandContext(ctx, "adb", "-s", companions[0].SerialNumber, "shell", "svc", "wifi", "enable").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to enable WiFi on the Android: ", err)
	}
	if err := d2.CommandContext(ctx, "adb", "-s", companions[0].SerialNumber, "shell", "cmd", "wifi", "connect-network", f.ssid, "wpa2", passphrase).Run(ssh.DumpLogOnError); err != nil {
		s.Fatalf("Failed to connect to SSID %s on Android: %s", f.ssid, err)
	}

	// Make sure Android phone is connected to the WiFi AP
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := d2.CommandContext(ctx, "adb", "-s", companions[0].SerialNumber, "shell", "cmd", "wifi", "status", "|", "grep", "'Wifi is connected to'").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to check for wifi status")
		}

		if out != nil {
			// Print current SSID
			if strings.Contains(string(out), f.ssid) {
				s.Logf("Android phone wifi status: %s", string(out))
				return nil
			}
		}

		return errors.New("Android phone is not yet connected to WiFi AP")
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		s.Fatal("Android phone is not connected to WiFi AP: ", err)
	}

	// Get the IP Address of the Android phone we want to setup adb-over-tcp with.
	// Filter out the cellular IP address, and poll for it to appear since it can take
	// a while after connecting to the wificell network.
	ipRegex := regexp.MustCompile(`([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})`)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err = d2.CommandContext(ctx, "adb", "-s", companions[0].SerialNumber, "shell", "ip", "route", "|", "grep", "-v", "rmnet", "|", "awk", "'{print $9}'").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to get Android phones IP Address")
		}
		if out != nil {
			if ipRegex.Match(out) {
				s.Log("IP Address result from ADB is: ", string(out))
				return nil
			}
		}
		return errors.New("Android phone is not yet connected to WiFi AP")
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: time.Second}); err != nil {
		s.Fatal("Failed to get Android's IP address on the wificell network: ", err)
	}

	phoneIPAddress := fmt.Sprintf("%s:%d", strings.TrimSpace(string(out)), 5555)
	s.Logf("Android IP Address is: %s", phoneIPAddress)

	// Storing adb connection information
	f.labstationConn = d2
	f.serialNumber = companions[0].SerialNumber

	versions, err := f.gmsCoreVersions(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve GMSCore versions: ", err)
	}
	s.Log("nearbyRemoteFixture.SetUp: GMSCore versions:", versions)

	s.Log("Disable GMSCore auto-updates by overriding phenotype flag")
	if err := f.overridePhenotypeFlag(ctx, "com.google.android.gms", "AutoUpdateCodegen__gms_auto_update_enabled", "false", "boolean"); err != nil {
		s.Fatal("Failed to override phenotype flag on Android phone: ", err)
	}

	// Enable adb-over-tcp on the Android device with the serial number we are using this test run.
	s.Log("Setting up adb-over-tcp on the Android device")
	if err = d2.CommandContext(ctx, "adb", "-s", companions[0].SerialNumber, "tcpip", "5555").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to enable adb-over-tcp on Android phone")
	}

	// Return phone IP Address to the local fixture.
	return []string{phoneIPAddress, f.ssid, passphrase}
}

func (f *nearbyRemoteFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.wf.CleanDisconnectWifi(ctx); err != nil {
		s.Error("Failed to disconnect WiFi, err: ", err)
	}
	disableInternetScript := "iptables -D FORWARD -i managed1 -o eth0 -j ACCEPT;" +
		"iptables -D FORWARD -i eth0 -o managed1 -m state --state ESTABLISHED,RELATED -j ACCEPT;" +
		"iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE;" +
		"iptables -D INPUT -i managed1 -j ACCEPT;" +
		"iptables -D INPUT -i eth0 -m state --state ESTABLISHED,RELATED -j ACCEPT;" +
		"iptables -D OUTPUT -j ACCEPT;" +
		"echo 1 > /proc/sys/net/ipv4/ip_forward"
	if err := f.wf.APConn().CommandContext(ctx, "sh", "-c", disableInternetScript).Run(ssh.DumpLogOnError); err != nil {
		s.Error("Fail to run iptables commands to disable Internet access: ", err)
	}
	if err := f.wf.DeconfigAP(ctx, f.apIface); err != nil {
		s.Error("Failed to deconfig ap, err: ", err)
	}
}
func (*nearbyRemoteFixture) Reset(ctx context.Context) error                        { return nil }
func (*nearbyRemoteFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (*nearbyRemoteFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

// overridePhenotypeFlag sets the specified Phenotype flag to the provided value for all accounts on the device.
func (f *nearbyRemoteFixture) overridePhenotypeFlag(ctx context.Context, pkg, flag, value, valueType string) error {
	err := f.labstationConn.CommandContext(ctx, "adb", "-s", f.serialNumber,
		"shell", "am", "broadcast", "-a", "com.google.android.gms.phenotype.FLAG_OVERRIDE",
		"--es", "package", pkg,
		"--es", "user", `*`,
		"--esa", "flags", flag,
		"--esa", "values", value,
		"--esa", "types", valueType,
		"com.google.android.gms").Run(ssh.DumpLogOnError)
	if err != nil {
		return errors.Errorf("failed to execute overridePhenotypeFlag for flag: %s", flag)
	}

	return err
}

// gmsCoreVersions returns both GMS Core versions on the phone.
// There will typically be two: the active running version, and a backup
// that is bundled with Android OS that is used when the device is factory reset.
func (f *nearbyRemoteFixture) gmsCoreVersions(ctx context.Context) ([]int, error) {

	versionInfo, err := f.labstationConn.CommandContext(ctx, "adb", "-s", f.serialNumber,
		"shell", "dumpsys", "package", "com.google.android.gms", "|", "grep", "versionCode").Output(ssh.DumpLogOnError)

	if err != nil {
		return nil, err
	}

	const versionCodePattern = "versionCode=([0-9]+)"
	r, err := regexp.Compile(versionCodePattern)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compile versionCode pattern")
	}
	versionCodeMatch := r.FindAllStringSubmatch(string(versionInfo), -1)
	if len(versionCodeMatch) == 0 {
		return nil, errors.New("GMS Core version number not found in command output")
	}
	var versions []int
	for _, v := range versionCodeMatch {
		version, err := strconv.Atoi(v[1])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert GMS Core version %v to int", v)
		}
		versions = append(versions, version)
	}
	return versions, nil
}
