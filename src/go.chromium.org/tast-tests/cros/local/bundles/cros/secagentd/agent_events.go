// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	rep "go.chromium.org/chromiumos/reporting"
	xdr "go.chromium.org/chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AgentEvents,
		Desc: "Checks that Agent Start Event is correctly being reported",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
			"rborzello@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security (XDR) > Alerts
		BugComponent: "b:1676923",
		Attr:         []string{"group:mainline", "group:enterprise-reporting"},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"bpf", "shipping_kernel"},
	})
}

// fillTpmRegexMap compiles the regex for each token. Called at init time.
func fillTpmRegexMap() (map[string]*regexp.Regexp, error) {
	tpmRegexMap := make(map[string]*regexp.Regexp)

	hexKeys := [4]string{"family", "manufacturer", "tpm_model", "firmware_version"}
	nonHexKeys := [3]string{"spec_level", "tpm_model", "firmware_version"}
	var err error

	for _, key := range hexKeys {
		// Matches hex string ignoring trailing pairs of zeros.
		if tpmRegexMap[key], err = regexp.Compile(key + `: .* \(0x([\dA-F]+?)(([0]{2})+\)|\))`); err != nil {
			return nil, err
		}
	}
	for _, key := range nonHexKeys {
		// Matches number.
		if tpmRegexMap[key], err = regexp.Compile(key + `: ([\d]+)`); err != nil {
			return nil, err
		}
	}

	// gsc_device and vendor_specific follow different pattern.
	// TODO(b/373640432): Convert `(?:gsc_device|gsc_version)` to `gsc_device`
	// after rename CL has been stable for a few builds.
	if tpmRegexMap["gsc_device"], err = regexp.Compile(`(?:gsc_device|gsc_version): (.*)`); err != nil {
		return nil, err
	}
	if tpmRegexMap["vendor_specific"], err = regexp.Compile("vendor_specific" + `: ([\dA-F]+)`); err != nil {
		return nil, err
	}

	return tpmRegexMap, nil
}

// makeExpectedAgent fills in the provided AgentStartEvent proto.
// Proto contents reflect what we expect the start event to look like.
func makeExpectedAgent(ctx context.Context, start *xdr.AgentStartEvent) error {
	start.Tcb = &xdr.TcbAttributes{}
	tcb := start.Tcb

	version, err := testexec.CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		return err
	}
	// Remove newline character.
	tcb.LinuxKernelVersion = proto.String(string(version[0 : len(version)-1]))

	// Gives error when value not set, so ignore error.
	fwid, _ := testexec.CommandContext(ctx, "crossystem", "fwid").Output()
	if len(fwid) != 0 {
		tcb.SystemFirmwareVersion = proto.String(string(fwid))
	}

	if err = fillTpmInformation(ctx, tcb); err != nil {
		return err
	}

	return nil
}

// fillTpmInformation fills out the tpm information in the proto.
func fillTpmInformation(ctx context.Context, tcb *xdr.TcbAttributes) error {
	out, err := testexec.CommandContext(ctx, "tpm_manager_client", "status").Output()
	if err != nil {
		return err
	}

	status := string(out)
	if !strings.Contains(status, "status: STATUS_SUCCESS") {
		return errors.New("TPM status request FAILED")
	}
	if !strings.Contains(status, "enabled: true") {
		return nil
	}

	out, err = testexec.CommandContext(ctx, "tpm_manager_client", "get_version_info").Output()
	if err != nil {
		return err
	}
	tpmInfo := string(out)
	if !strings.Contains(tpmInfo, "status: STATUS_SUCCESS") {
		return errors.New("TPM info request FAILED")
	}

	tcb.SecurityChip = &xdr.TcbAttributes_SecurityChip{}
	chip := tcb.SecurityChip

	tpmRegexMap, err := fillTpmRegexMap()
	if err != nil {
		return err
	}

	re := tpmRegexMap["gsc_device"]
	gsc := re.FindStringSubmatch(tpmInfo)[1]

	switch gsc {
	case "GSC_DEVICE_NOT_GSC":
		chip.Kind = xdr.TcbAttributes_SecurityChip_TPM.Enum()
	case "GSC_DEVICE_DT", "GSC_DEVICE_H1", "GSC_DEVICE_NT":
		chip.Kind = xdr.TcbAttributes_SecurityChip_GOOGLE_SECURITY_CHIP.Enum()
	// TODO(b/373640432) Remove GSC_VERSION arms after rename CL has been stable
	// for a few builds.
	case "GSC_VERSION_NOT_GSC":
		chip.Kind = xdr.TcbAttributes_SecurityChip_TPM.Enum()
	case "GSC_VERSION_CR50", "GSC_VERSION_TI50":
		chip.Kind = xdr.TcbAttributes_SecurityChip_GOOGLE_SECURITY_CHIP.Enum()
	default:
		return errors.Errorf("Unrecognized gsc_device: %q", gsc)
	}

	family, err := getTpmValue("family", tpmInfo, true, tpmRegexMap)
	if err != nil {
		return err
	}
	chip.SpecFamily = proto.String(family)

	value, _ := getTpmValue("spec_level", tpmInfo, false, tpmRegexMap)
	level, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}
	specLevel := strconv.FormatInt((level>>32)&0xffffffff, 10)
	revision := strconv.FormatInt(level&0xffffffff, 10)
	chip.SpecLevel = proto.String(specLevel)

	chip.ChipVersion = proto.String(fmt.Sprintf("%s.%s.%s", family, specLevel, revision))
	value, err = getTpmValue("manufacturer", tpmInfo, true, tpmRegexMap)
	if err != nil {
		return err
	}
	chip.Manufacturer = proto.String(value)

	value, _ = getTpmValue("tpm_model", tpmInfo, false, tpmRegexMap)
	chip.TpmModel = proto.String(value)

	value, _ = getTpmValue("firmware_version", tpmInfo, false, tpmRegexMap)
	chip.FirmwareVersion = proto.String(value)

	value, err = getTpmValue("vendor_specific", tpmInfo, true, tpmRegexMap)
	if err != nil {
		return err
	}
	chip.VendorId = proto.String(value)

	return nil
}

// getTpmValue finds the corresponding value for different tpm fields.
func getTpmValue(key, tpmInfo string, hexString bool, tpmRegexMap map[string]*regexp.Regexp) (string, error) {
	re := tpmRegexMap[key]
	matches := re.FindStringSubmatch(tpmInfo)
	// Check that field exists.
	if len(matches) < 2 {
		return "", nil
	}

	var value string
	if hexString {
		out, err := hex.DecodeString(matches[1])
		if err != nil {
			return "", err
		}
		value = string(out)
	} else {
		value = matches[1]
	}

	return value, nil
}

// AgentEvents starts secagentd and verifies that the AgentStart is only sent.
func AgentEvents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer func(ctx context.Context) {
		secagentdupstart.RestartSecagentd(ctx, false)
		cancel()
	}(cleanupCtx)

	// Restart secagentd, have it ignore policy.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx, false,
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"))
	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	ew, cancel, err := secagentddbusmonitor.SetupDbusWatcherWithTimeout(ctx, agentPid, 30*time.Second, /*waitForBpf=*/false)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}
	defer cancel()

	expAgent := xdr.AgentStartEvent{}
	if err = makeExpectedAgent(ctx, &expAgent); err != nil {
		s.Fatal("Failed to make expected AgentEvent proto: ", err)
	}

	event, ok := <-ew.Events()
	if !ok {
		s.Fatal("Timed out waiting for expected events")
	}
	arg, ok := event.Arguments[0].([]byte)
	if !ok {
		s.Fatal("Failed to cast arguments")
	}
	enq := &rep.EnqueueRecordRequest{}
	if err := proto.Unmarshal(arg, enq); err != nil {
		s.Fatal("Failed to unmarshal an EnqueueRecordRequest: ", err)
	}
	if enq.GetRecord().GetDestination() != rep.Destination_CROS_SECURITY_AGENT {
		s.Fatalf("Wrong destination: got %s, want CROS_SECURITY_AGENT", enq.GetRecord().GetDestination())
	}
	pe := &xdr.XdrAgentEvent{}
	if err := proto.Unmarshal(enq.GetRecord().GetData(), pe); err != nil {
		s.Fatal("Failed to unmarshal data for a CROS_SECURITY_AGENT record: ", err)
	}
	if len(pe.GetBatchedEvents()) != 1 {
		s.Fatalf("Incorrect number of events: got %d, want 1", len(pe.GetBatchedEvents()))
	}

	batched := pe.GetBatchedEvents()[0]
	if err := secagentdcommon.CheckCommon(batched.GetCommon()); err != nil {
		s.Error("Invalid common field: ", err)
	}

	agent := batched.GetAgentStart()
	//TODO(b/254534567) Source secureboot for expected AgentStart. Copy for now.
	if agent.Tcb.FirmwareSecureBoot != nil {
		expAgent.Tcb.FirmwareSecureBoot = agent.Tcb.GetFirmwareSecureBoot().Enum()
	}
	if !proto.Equal(&expAgent, agent) {
		s.Log("Actual AgentStart: ", agent.String())
		s.Log("Expected AgentStart: ", expAgent.String())
		s.Error("Agent Events FAILED to match")
	}
}
