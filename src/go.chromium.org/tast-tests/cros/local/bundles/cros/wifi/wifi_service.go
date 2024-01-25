// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			wifiService := Service{sharedObject: common.SharedObjectsForServiceSingleton, serviceState: s}
			wifi.RegisterWifiServiceServer(srv, &wifiService)
		},
	})
}

type Service struct {
	serviceState *testing.ServiceState
	sharedObject *common.SharedObjectsForService
}

// runtimeResources holds the resources that needs to be retrieved in runtime,
// such as connections that are associated with Chrome session.
type runtimeResources struct {
	cr    *chrome.Chrome
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// initializeRuntimeResources initializes all service-scoped resources.
func (s *Service) initializeRuntimeResources(ctx context.Context) (*runtimeResources, error) {
	// DUT could be logged and out, hence, it shouldn't be held as service-scoped resource.
	cr := s.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}

	// A Test API connection is associated with Chrome session, it shouldn't be held as service-scoped resource either.
	tconn, err := common.UseTconn(ctx, s.sharedObject, func(tconn *chrome.TestConn) (*chrome.TestConn, error) {
		return tconn, nil
	})
	if err != nil {
		return nil, err
	}

	return &runtimeResources{
		cr:    cr,
		tconn: tconn,
		ui:    uiauto.New(tconn),
	}, nil
}

func (s *Service) JoinWifiFromQuickSettings(ctx context.Context, req *wifi.JoinWifiRequest) (_ *emptypb.Empty, retErr error) {
	res, err := s.initializeRuntimeResources(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := quicksettings.NavigateToNetworkDetailedView(ctx, res.tconn); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to navigate to network detailed view within the Quick Settings")
	}
	defer quicksettings.Hide(cleanupCtx, res.tconn)
	defer s.dumpUITreeWithScreenshotOnError(cleanupCtx, func() bool { return retErr != nil }, "quick_settings_ui_dump")

	joinWiFiButton := nodewith.Name("Join Wi-Fi network").Role(role.Button).Ancestor(quicksettings.QsRootFinder)
	// DoDefault is a better choice, as the button is at the bottom of the network list,
	// which the list change whenever the available network list change, makes the click action highly unstable.
	if err := res.ui.DoDefault(joinWiFiButton)(ctx); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to click the join button")
	}

	joinWiFiNetworkDialog := nodewith.NameContaining("Join Wi-Fi network").Role(role.Dialog)
	if err := res.ui.WaitUntilExists(joinWiFiNetworkDialog)(ctx); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, `failed to find the "Join Wi-Fi" dialog`)
	}

	ssidTextField := nodewith.NameContaining("SSID").Role(role.TextField).Ancestor(joinWiFiNetworkDialog)
	if err := setTextField(res.ui, kb, ssidTextField, req.GetSsid())(ctx); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to fill the SSID")
	}

	if err := joinWifiSetSecurity(res.tconn, res.ui, kb, req, joinWiFiNetworkDialog)(ctx); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to set security")
	}

	return &emptypb.Empty{}, verifyConnectedStatus(ctx, req.Ssid, true)
}

func joinWifiSetSecurity(tconn *chrome.TestConn, ui *uiauto.Context, kb *input.KeyboardEventWriter, req *wifi.JoinWifiRequest, joinWiFiNetworkDialogRoot *nodewith.Finder) uiauto.Action {
	var securityOptionName string
	var authenticateAction uiauto.Action
	switch req.Security.(type) {
	case *wifi.JoinWifiRequest_None:
		securityOptionName = "None"
		authenticateAction = func(ctx context.Context) error { return nil }
	case *wifi.JoinWifiRequest_Psk:
		securityOptionName = "PSK (WPA or RSN)"
		passwordTextField := nodewith.Name("Password").Role(role.TextField).Ancestor(joinWiFiNetworkDialogRoot)
		authenticateAction = setTextField(ui, kb, passwordTextField, req.GetPsk())
	case *wifi.JoinWifiRequest_EapTls:
		securityOptionName = "EAP"
		eapMethodComboBox := nodewith.Name("EAP method").Role(role.ComboBoxSelect).Ancestor(joinWiFiNetworkDialogRoot)
		caComboBox := nodewith.Name("Server CA certificate").Role(role.ComboBoxSelect).Ancestor(joinWiFiNetworkDialogRoot)
		userComboBox := nodewith.Name("User certificate").Role(role.ComboBoxSelect).Ancestor(joinWiFiNetworkDialogRoot)
		identityTextField := nodewith.Name("Identity").Role(role.TextField).Ancestor(joinWiFiNetworkDialogRoot)
		authenticateAction = uiauto.Combine("set EAP certificates",
			dropdown.SelectDropDownOption(tconn, eapMethodComboBox, "EAP-TLS"),
			dropdown.SelectDropDownOption(tconn, caComboBox, req.GetEapTls().GetCaCert()),
			dropdown.SelectDropDownOption(tconn, userComboBox, req.GetEapTls().GetClientCert()),
			// Any non-empty string would work for EAP-TLS.
			setTextField(ui, kb, identityTextField, "test"),
		)
	}
	var setAsSharedAction uiauto.Action
	switch req.ShareWithOtherUsers {
	case wifi.JoinWifiRequest_Default:
		setAsSharedAction = func(ctx context.Context) error { return nil }
	case wifi.JoinWifiRequest_TurnOn, wifi.JoinWifiRequest_TurnOff:
		expected := checked.False
		if req.ShareWithOtherUsers == wifi.JoinWifiRequest_TurnOn {
			expected = checked.True
		}
		toggleButton := nodewith.Name("Allow other users of this device to use this network").Role(role.ToggleButton).Ancestor(joinWiFiNetworkDialogRoot)
		setAsSharedAction = func(ctx context.Context) error {
			if err := uiauto.Combine("make visiable and wait until the node is stable",
				ui.EnsureFocused(toggleButton),
				ui.WaitForLocation(toggleButton),
			)(ctx); err != nil {
				return err
			}

			if info, err := ui.Info(ctx, toggleButton); err != nil {
				return errors.Wrap(err, "failed to get node info")
			} else if info.Checked != expected {
				return ui.DoDefault(toggleButton)(ctx)
			}
			return nil
		}
	}

	securityComboBox := nodewith.Name("Security").Role(role.ComboBoxSelect).Ancestor(joinWiFiNetworkDialogRoot)
	connectButton := nodewith.NameContaining("Connect").Role(role.Button).Ancestor(joinWiFiNetworkDialogRoot)
	return uiauto.Combine("set security and connect to wifi "+req.Ssid,
		dropdown.SelectDropDownOption(tconn, securityComboBox, securityOptionName),
		authenticateAction,
		setAsSharedAction,
		ui.LeftClick(connectButton),
	)
}

func (s *Service) KnownNetworksControls(ctx context.Context, req *wifi.KnownNetworksControlsRequest) (_ *emptypb.Empty, retErr error) {
	res, err := s.initializeRuntimeResources(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	const pageShortURL = "knownNetworks"
	condition := res.ui.Exists(nodewith.NameContaining("Known Networks").Role(role.Heading).Ancestor(ossettings.WindowFinder))
	settings, err := ossettings.LaunchAtPageURL(ctx, res.tconn, res.cr, pageShortURL, condition)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to launch OS Settings and navigate to the specific page")
	}
	defer settings.Close(cleanupCtx)
	defer s.dumpUITreeWithScreenshotOnError(cleanupCtx, func() bool { return retErr != nil }, "known_networks_controls")

	for _, ssid := range req.Ssids {
		networkItem := nodewith.Name(ssid).Role(role.Link)

		switch req.Control {
		case wifi.KnownNetworksControlsRequest_WaitUntilExist:
			if err := settings.WaitUntilExists(networkItem)(ctx); err != nil {
				return &emptypb.Empty{}, err
			}
		case wifi.KnownNetworksControlsRequest_WaitUntilGone:
			if err := uiauto.Combine("ensure Wifi finder gone",
				settings.WaitUntilGone(networkItem),
				settings.EnsureGoneFor(networkItem, 5*time.Second),
			)(ctx); err != nil {
				return &emptypb.Empty{}, err
			}
		case wifi.KnownNetworksControlsRequest_Forget:
			if err := uiauto.Combine(fmt.Sprintf("forget network %q", ssid),
				settings.LeftClick(nodewith.Name("More actions for "+ssid).Role(role.Button)),
				settings.LeftClick(nodewith.Name("Forget").Role(role.MenuItem)),
				settings.WaitUntilGone(networkItem),
			)(ctx); err != nil {
				return &emptypb.Empty{}, err
			}
			if err := verifyConnectedStatus(ctx, ssid, false); err != nil {
				return &emptypb.Empty{}, err
			}
		case wifi.KnownNetworksControlsRequest_Disconnect:
			if err := uiauto.Combine(fmt.Sprintf("disconnect network %q", req.Ssids),
				settings.LeftClick(networkItem),
				settings.LeftClick(nodewith.Name("Disconnect").Role(role.Button)),
				settings.WaitUntilExists(nodewith.Name("Not Connected").Role(role.StaticText)),
				settings.WaitUntilExists(nodewith.Name("Connect").Role(role.Button)),
			)(ctx); err != nil {
				return &emptypb.Empty{}, err
			}
			// The disconnect control clicks on the network and navigate to another page.
			// Need to navigate back to "Known Networks" for the next iteration.
			if err := settings.NavigateToPageURL(ctx, res.cr, pageShortURL, condition); err != nil {
				return &emptypb.Empty{}, err
			}
		case wifi.KnownNetworksControlsRequest_Connect:
			if err := settings.LeftClick(networkItem)(ctx); err != nil {
				return &emptypb.Empty{}, err
			}

			// Skip the connection attempt if we are already connected.
			if err := verifyConnectedStatus(ctx, ssid, true); err != nil {
				if err := uiauto.Combine("connect to known network",
					settings.LeftClick(nodewith.Name("Connect").Role(role.Button)),
					waitUntilConnected(res.tconn, settings),
				)(ctx); err != nil {
					return &emptypb.Empty{}, err
				}
			}

			// The connect control clicks on the network and navigate to another page.
			// Need to navigate back to "Known Networks" for the next iteration.
			if err := settings.NavigateToPageURL(ctx, res.cr, pageShortURL, condition); err != nil {
				return &emptypb.Empty{}, err
			}
		case wifi.KnownNetworksControlsRequest_ShownAsShared:
			// The label will be "Other users on this device can also use this network" for primary user,
			// and "This network is shared with you" for other users.
			// Both of them indicate this network is shared.
			r := regexp.MustCompile(`(Other users on this device can also use this network|This network is shared with you)`)

			if err := uiauto.Combine("check network is shown as shared",
				settings.LeftClick(networkItem),
				settings.WaitUntilExists(nodewith.NameRegex(r).Role(role.StaticText)),
			)(ctx); err != nil {
				return &emptypb.Empty{}, err
			}
			// The 'ShownAsShared' control clicks on the network and navigate to another page.
			// Need to navigate back to "Known Networks" for the next iteration.
			if err := settings.NavigateToPageURL(ctx, res.cr, pageShortURL, condition); err != nil {
				return &emptypb.Empty{}, err
			}
		default:
			return &emptypb.Empty{}, errors.Errorf("unrecognized control type %d", req.Control)
		}
	}

	return &emptypb.Empty{}, nil
}

// WifiPageControl opens the OS-Settings at "Wi-Fi" page and interacts/controls the WiFi networks.
func (s *Service) WifiPageControl(ctx context.Context, req *wifi.WifiPageControlRequest) (_ *emptypb.Empty, retErr error) {
	res, err := s.initializeRuntimeResources(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	settings, err := ossettings.LaunchAtWiFi(ctx, res.tconn, res.cr)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to launch OS Settings and navigate to the specific page")
	}
	defer settings.Close(cleanupCtx)
	defer s.dumpUITreeWithScreenshotOnError(cleanupCtx, func() bool { return retErr != nil }, "check_wifi_settings_ui_dump")

	wifiNameRegex := regexp.MustCompile(fmt.Sprintf(`^Network (\d)+ of (\d)+, %s`, req.Ssid))
	wifiFinder := nodewith.NameRegex(wifiNameRegex).HasClass("layout")
	switch req.Control {
	case wifi.WifiPageControlRequest_WaitUntilExist:
		if err := settings.WaitUntilExists(wifiFinder)(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to wait until WiFi node exists")
		}
	case wifi.WifiPageControlRequest_WaitUntilGone:
		if err := settings.WaitUntilGone(wifiFinder)(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to wait until WiFi node gone")
		}
	case wifi.WifiPageControlRequest_WaitUntilConnected:
		connected := nodewith.NameStartingWith("Connected").Role(role.StaticText)
		if err := settings.WaitUntilExists(connected.Ancestor(wifiFinder))(ctx); err != nil {
			return &emptypb.Empty{}, errors.Wrap(err, "failed to wait until WiFi connected")
		}
	default:
		return &emptypb.Empty{}, errors.Errorf("unrecognized control type %d", req.Control)
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) dumpUITreeWithScreenshotOnError(ctx context.Context, hasError func() bool, filePrefix string) {
	res, err := s.initializeRuntimeResources(ctx)
	if err != nil {
		s.serviceState.Log("Failed to dump UI tree with screenshot: ", err)
		return
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.serviceState.Log(ctx, "Failed to obtain output directory")
		return
	}

	if res.cr.LoginMode() == "NoLogin" {
		faillog.DumpUITreeOnErrorToFile(ctx, filepath.Join(outDir, "service"), hasError, res.tconn, filePrefix)
	} else {
		faillog.DumpUITreeWithScreenshotOnError(ctx, filepath.Join(outDir, "service"), hasError, res.cr, filePrefix)
	}
}

func verifyConnectedStatus(ctx context.Context, ssid string, expectedStatus bool) error {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a shill manager")
	}

	expectProps := map[string]interface{}{
		shillconst.ServicePropertyName:        ssid,
		shillconst.ServicePropertyIsConnected: expectedStatus,
	}
	if _, err := m.WaitForServiceProperties(ctx, expectProps, 15*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait WiFi to be expected status")
	}

	return nil
}

func setTextField(ui *uiauto.Context, kb *input.KeyboardEventWriter, textField *nodewith.Finder, text string) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("set text field with text: %q", text),
		ui.EnsureFocused(textField),
		kb.AccelAction("ctrl+A"),
		kb.AccelAction("backspace"),
		kb.TypeAction(text),
	)
}

func waitUntilConnected(tconn *chrome.TestConn, settings *ossettings.OSSettings) uiauto.Action {
	return func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			if found, err := settings.IsNodeFound(ctx, nodewith.NameStartingWith("Connected").Role(role.StaticText)); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to check if network is not connected"))
			} else if !found {
				return errors.New("network is not connected")
			}
			return nil
		}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: 300 * time.Millisecond})
	}
}
