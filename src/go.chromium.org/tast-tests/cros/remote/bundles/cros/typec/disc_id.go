// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast-tests/cros/remote/typec/typecswitch"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DiscID,
		Desc:     "Check that a device can be connected and verify disc ID",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Attr:         []string{"group:typec", "typec_unigraf274", "typec_informational"},
		Fixture:      "typecSwitch",
		Params: []testing.Param{{
			Val: typecswitch.TestSetupData{
				ConnectionMode: usbswitch.DpMode,
				Iterations:     100,
			},
			Timeout: 45 * time.Minute,
		}},
	})
}

// identityParams holds the expected values for a TypeC identity check.
type identityParams struct {
	certStat        string
	idHeader        string
	product         string
	productTypeVDO1 string
	productTypeVDO2 string
	productTypeVDO3 string
}

// modeParams holds the expected SVID and VDO for a TypeC mode check.
type modeParams struct {
	svid string
	vdo  string
}

// discIDParams holds all parameters for partner and cable discovery identity checks.
type discIDParams struct {
	partnerIdentity identityParams
	partnerModes    []modeParams
	cableIdentity   identityParams
	cableModes      []modeParams
}

// hpG4DiscIDParams contains parameters for an HP G4 dock.
var hpG4DiscIDParams = discIDParams{
	partnerIdentity: identityParams{
		certStat:        "0x00000000",
		idHeader:        "0x4ce003f0",
		product:         "0x04880083",
		productTypeVDO1: "0x6d80003b",
		productTypeVDO2: "0x00000000",
		productTypeVDO3: "0x40800000",
	},
	partnerModes: []modeParams{
		{svid: "ff01", vdo: "0x001c0045"},
		{svid: "8087", vdo: "0x04000001"},
		{svid: "03f0", vdo: "0x60000000"},
	},
	cableIdentity: identityParams{
		certStat:        "0x00000000",
		idHeader:        "0x1c6003f0",
		product:         "0x94880001",
		productTypeVDO1: "0x11082043",
		productTypeVDO2: "0x00000000",
		productTypeVDO3: "0x00000000",
	},
	cableModes: []modeParams{
		{svid: "1e4e", vdo: "0x9031011c"},
		{svid: "8087", vdo: "0x00030001"},
	},
}

// unigrafDiscIDParams contains parameters for Unigraf.
var unigrafDiscIDParams = discIDParams{
	partnerIdentity: identityParams{
		certStat:        "0xf0000003",
		idHeader:        "0x550016a6",
		product:         "0x0002aaaa",
		productTypeVDO1: "0x6400004a",
		productTypeVDO2: "0x00000000",
		productTypeVDO3: "0x00000000",
	},
	partnerModes: []modeParams{
		{svid: "ff01", vdo: "0x001c0045"},
	},
	// Currently Unigraf setup does not have active cable.
	cableIdentity: identityParams{
		certStat:        "0x00000000",
		idHeader:        "0x00000000",
		product:         "0x00000000",
		productTypeVDO1: "0x00000000",
		productTypeVDO2: "0x00000000",
		productTypeVDO3: "0x00000000",
	},
	cableModes: []modeParams{},
}

// DiscID does the following:
//
// - Disconnect the dock via switch interface.
// - Reconnect the dock via switch interface.
// - Verify that the dock is connected and all the disc ID parameters are correct.
//
// This test expects the following hardware topology:
//
//	 ____network___
//	|              |
//	|              |
//	Host          DUT ----- USB Switch ---- Dock
//	|                            |
//	|____________________________|
func DiscID(ctx context.Context, s *testing.State) {
	var params discIDParams

	d := s.DUT()
	testData := s.Param().(typecswitch.TestSetupData)
	s.Log("Number of iterations: ", testData.Iterations)

	// Get the switch from the fixture.
	fixtData, ok := s.FixtValue().(*typecswitch.FixtureData)
	if !ok {
		s.Fatal("Failed to get fixture data")
	}
	sw := fixtData.TestSwitch

	if err := typecswitch.SetupSwitch(ctx, sw, testData); err != nil {
		s.Fatal("Failed to setup switch: ", err)
	}

	// Select parameters based on the switch type.
	switch sw.GetType() {
	case usbswitch.UTC274:
		params = unigrafDiscIDParams
		s.Log("Detected Unigraf UTC-274, using Unigraf parameters")
	case usbswitch.Mcci:
		params = hpG4DiscIDParams // Use HP G4 for MCCI.
		s.Log("Detected MCCI switch, using HP G4 parameters")
	default:
		s.Fatal("Unknown or unsupported switch type")
	}
	s.Logf("Using Disc ID parameters: %+v", params)

	for i := 1; i <= testData.Iterations; i++ {
		s.Log("Running iteration ", i)
		if err := performDiscIDIteration(ctx, d, sw, params); err != nil {
			s.Fatalf("Failed test on iteration %d: %v", i, err)
		}
	}
}

const (
	typecPath  = "/sys/class/typec"
	portNumber = "1"
)

// performDiscIDIteration runs 1 iteration of the Disc ID test using the provided parameters.
func performDiscIDIteration(ctx context.Context, d *dut.DUT, sw usbswitch.Switch, params discIDParams) error {
	// Enable the port.
	if err := sw.EnablePort(ctx); err != nil {
		return errors.Wrap(err, "failed to enable the port")
	}

	// Wait for connection.
	if err := waitForConnection(ctx, d); err != nil {
		return errors.Wrap(err, "port check failed")
	}

	// Some fields need extra time to be filed
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Check partner.
		if err := checkPartner(ctx, d, params.partnerIdentity, params.partnerModes); err != nil {
			return errors.Wrap(err, "partner check failed")
		}
		// Check cable.
		if err := checkCable(ctx, d, params.cableIdentity, params.cableModes); err != nil {
			return errors.Wrap(err, "cable check failed")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		return err
	}

	// Disable the port.
	if err := sw.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to disable the port")
	}

	// Wait for disconnection.
	if err := waitForDisconnection(ctx, d); err != nil {
		return errors.Wrap(err, "disconnection check failed")
	}

	return nil
}

func waitForConnection(ctx context.Context, d *dut.DUT) error {
	partnerPath := filepath.Join(typecPath, "port"+portNumber+"-partner")
	cablePath := filepath.Join(typecPath, "port"+portNumber+"-cable")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := d.Conn().CommandContext(ctx, "test", "-e", partnerPath).CombinedOutput(); err != nil {
			return errors.Wrapf(err, "partner is not present: %s", partnerPath)
		}
		if _, err := d.Conn().CommandContext(ctx, "test", "-e", cablePath).CombinedOutput(); err != nil {
			return errors.Wrapf(err, "cable is not present: %s", cablePath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 4 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		return err
	}
	return nil
}

func waitForDisconnection(ctx context.Context, d *dut.DUT) error {
	partnerPath := filepath.Join(typecPath, "port"+portNumber+"-partner")
	cablePath := filepath.Join(typecPath, "port"+portNumber+"-cable")

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := d.Conn().CommandContext(ctx, "test", "!", "-e", partnerPath).CombinedOutput(); err != nil {
			return errors.Wrapf(err, "partner is still present: %s", partnerPath)
		}
		if _, err := d.Conn().CommandContext(ctx, "test", "!", "-e", cablePath).CombinedOutput(); err != nil {
			return errors.Wrapf(err, "cable is still present: %s", cablePath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 4 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		return err
	}
	return nil
}

// checkPartner verifies the partner identity and modes using the provided parameters.
func checkPartner(ctx context.Context, d *dut.DUT, identity identityParams, modes []modeParams) error {
	partnerPath := filepath.Join(typecPath, "port"+portNumber+"-partner")
	identityPath := filepath.Join(partnerPath, "identity")

	// Check identity using fields from the identity struct.
	if err := checkIdentity(ctx, d, identityPath, identity); err != nil {
		return errors.Wrap(err, "partner identity check failed")
	}

	// Check modes using the modes slice.
	for i, mode := range modes {
		modePath := filepath.Join(partnerPath, fmt.Sprintf("port%s-partner.%d", portNumber, i))
		if err := checkMode(ctx, d, modePath, mode.svid, mode.vdo); err != nil {
			return errors.Wrapf(err, "partner mode (%d) check failed", i)
		}
	}

	// Verify that no extra modes exist.
	nextModeIndex := len(modes)
	nextModePath := filepath.Join(partnerPath, fmt.Sprintf("port%s-partner.%d", portNumber, nextModeIndex))
	if _, err := d.Conn().CommandContext(ctx, "test", "-e", nextModePath).CombinedOutput(); err == nil {
		return errors.Errorf("unexpected partner mode (%d) found at %s", nextModeIndex, nextModePath)
	}

	return nil
}

// checkCable verifies the cable identity and modes using the provided parameters.
func checkCable(ctx context.Context, d *dut.DUT, identity identityParams, modes []modeParams) error {
	cablePath := filepath.Join(typecPath, "port"+portNumber+"-cable")
	identityPath := filepath.Join(cablePath, "identity")

	// Check identity using fields from the identity struct.
	if err := checkIdentity(ctx, d, identityPath, identity); err != nil {
		return errors.Wrap(err, "cable identity check failed")
	}

	// Check modes using the modes slice.
	// Note: Cable modes are associated with plug0.
	for i, mode := range modes {
		// Use the base typecPath for cable modes, not the cablePath itself.
		modePath := filepath.Join(typecPath, fmt.Sprintf("port%s-plug0.%d", portNumber, i))
		if err := checkMode(ctx, d, modePath, mode.svid, mode.vdo); err != nil {
			return errors.Wrapf(err, "cable mode (%d) check failed", i)
		}
	}

	// Verify that no extra modes exist.
	nextModeIndex := len(modes)
	nextModePath := filepath.Join(typecPath, fmt.Sprintf("port%s-plug0.%d", portNumber, nextModeIndex))
	if _, err := d.Conn().CommandContext(ctx, "test", "-e", nextModePath).CombinedOutput(); err == nil {
		return errors.Errorf("unexpected cable mode (%d) found at %s", nextModeIndex, nextModePath)
	}

	return nil
}

func checkIdentity(ctx context.Context, d *dut.DUT, identityPath string, identity identityParams) error {
	if err := checkFileContent(ctx, d, filepath.Join(identityPath, "cert_stat"), identity.certStat); err != nil {
		return err
	}
	if err := checkFileContent(ctx, d, filepath.Join(identityPath, "id_header"), identity.idHeader); err != nil {
		return err
	}
	if err := checkFileContent(ctx, d, filepath.Join(identityPath, "product"), identity.product); err != nil {
		return err
	}
	if err := checkFileContent(ctx, d, filepath.Join(identityPath, "product_type_vdo1"), identity.productTypeVDO1); err != nil {
		return err
	}
	if err := checkFileContent(ctx, d, filepath.Join(identityPath, "product_type_vdo2"), identity.productTypeVDO2); err != nil {
		return err
	}
	if err := checkFileContent(ctx, d, filepath.Join(identityPath, "product_type_vdo3"), identity.productTypeVDO3); err != nil {
		return err
	}
	return nil
}

func checkMode(ctx context.Context, d *dut.DUT, modePath, svid, vdo string) error {
	if err := checkFileContent(ctx, d, filepath.Join(modePath, "svid"), svid); err != nil {
		return err
	}
	if err := checkFileContent(ctx, d, filepath.Join(modePath, "vdo"), vdo); err != nil {
		return err
	}
	return nil
}

func checkFileContent(ctx context.Context, d *dut.DUT, filePath, expectedContent string) error {
	out, err := d.Conn().CommandContext(ctx, "cat", filePath).Output()
	if err != nil {
		return errors.Wrapf(err, "couldn't read file %s", filePath)
	}

	if strings.TrimSpace(string(out)) != expectedContent {
		return errors.Errorf("unexpected content in %s: got %q, want %q", filePath, string(out), expectedContent)
	}
	return nil
}
