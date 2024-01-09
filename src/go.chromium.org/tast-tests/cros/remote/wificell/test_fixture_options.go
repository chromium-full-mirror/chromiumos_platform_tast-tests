// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"encoding/json"
	"fmt"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TFOptions are fixture options that may be configured by a TFOptionsBuilder.
// These values persist and are constant once NewTestFixture is completed.
//
// Should only be created using a TFOptionsBuilder, not be instantiated directly.
type TFOptions struct {
	// DutTargets are the hostnames of the duts to use with the fixture.
	DutTargets []string

	// duts stores the dut.DUT and testing.RPCHint for each dut in DutTargets as
	// they are passed in TFOptionDutTargets for later use in TestFixture.
	duts []*dutData

	// PrimaryRouterTargets are the hostnames of the routers to use as the primary
	// routers.
	//
	// Leave empty to use one router with the default router hostname (<dut>-router).
	PrimaryRouterTargets []string

	// PcapRouterTarget is the hostname of the router to use as the pcap router.
	//
	// Only used if EnablePacketCapture is true.
	//
	// Leave unset to use the default pcap hostname (<dut>-pcap>). Will revert
	// to using the first router as a pcap if this fails.
	//
	// If also in PrimaryRouterTargets, this has the same effect of UseFirstRouterAsPcap.
	PcapRouterTarget string

	// AttenuatorTarget is the hostname of the attenuator to use with the first
	// primary router.
	//
	// Leave unset if no attenuator present or to not use it.
	AttenuatorTarget string

	// HostUserOverrides is map of hostnames to usernames to use for host targets
	// instead of the default "root" user. Useful for AsusWrt router hosts, as
	// they require the "admin" user.
	HostUserOverrides map[string]string

	// RequirePrimaryRouter when true, will require at least one primary router to
	// be configured.
	//
	// Default is true.
	RequirePrimaryRouter bool

	// EnablePacketCapture when true, will configure a router as a pcap.
	//
	// Default is false.
	EnablePacketCapture bool

	// EnableBridgeAndVeth when true, will configure bridge and veth between
	// two WLAN interfaces in a router.
	//
	// Default is false.
	EnableBridgeAndVeth bool

	// UseFirstRouterAsPcap when true, will use the first primary router also as
	// the pcap. This can have limited packet capturing abilities, so only enable
	// this when specifically needed by a test.
	// Default is false.
	UseFirstRouterAsPcap bool

	// EnableRouterReboot when true, will cause routers to be rebooted between
	// tests to more reliably clean their state.
	//
	// Only OpenWrt routers are rebooted.
	//
	// Default is true.
	//
	// Can be helpful to disable this to allow for easier test debugging and
	// development. Should only be disabled when necessary, not for normal test
	// runs.
	EnableRouterReboot bool

	// EnableDutUI when true, will not skip stopping the UI on the duts when
	// initializing Chrome.
	//
	// This Options is useful for tests with UI settings + basic WiFi
	// functionality, where the interference of UI (e.g. trigger scans) does not
	// matter much.
	//
	// Default is false.
	EnableDutUI bool

	// EnableCellular when true, will initialize a dut client for
	// cellular.RemoteCellularService  to assist in cellular testing.
	//
	// Default is false.
	EnableCellular bool

	// SetDutWifiLogging when true, will configure the logging of the wifi service
	// on the duts (via shill) with the settings configured by DutWifiLogLevel
	// and DutWifiLogTags.
	//
	// Default is true.
	SetDutWifiLogging bool

	// DutWifiLogLevel is the log level of the wifi service on the duts that is
	// configured when SetDutWifiLogging is true.
	//
	// Default is -2.
	DutWifiLogLevel int

	// DutWifiLogTags are the log tags the wifi service on the duts that is
	// configured when SetDutWifiLogging is true.
	//
	// Default is ["wifi"].
	DutWifiLogTags []string
}

// newTFOptions creates a new TFOptions with all default values populated.
func newTFOptions() *TFOptions {
	// Set all public fields, including those that are technically the same as
	// the default field value to be verbose.
	return &TFOptions{
		DutTargets:           nil,
		PrimaryRouterTargets: nil,
		PcapRouterTarget:     "",
		AttenuatorTarget:     "",
		HostUserOverrides:    nil,
		RequirePrimaryRouter: true,
		EnablePacketCapture:  false,
		EnableBridgeAndVeth:  false,
		UseFirstRouterAsPcap: false,
		EnableRouterReboot:   true,
		EnableDutUI:          false,
		EnableCellular:       false,
		SetDutWifiLogging:    true,
		DutWifiLogLevel:      -2,
		DutWifiLogTags:       []string{"wifi"},
	}
}

// String returns options as a pretty JSON string.
func (o *TFOptions) String() string {
	optionsJSON, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return fmt.Sprintf("%p", &o)
	}
	return string(optionsJSON)
}

// Validate will check options that depend upon each other and throw an error
// if anything is misconfigured.
func (o *TFOptions) Validate() error {
	if len(o.DutTargets) == 0 {
		return errors.New("at least one dut target is required")
	}
	if len(o.DutTargets) != len(o.duts) {
		return errors.New("dut data not present for all dut targets; make sure you build TFOptions with a TFOptionsBuilder")
	}
	if o.PcapRouterTarget != "" && o.UseFirstRouterAsPcap {
		return errors.New("cannot set a PcapRouterTarget and have UseFirstRouterAsPcap as true")
	}
	return nil
}

// TFOptionsBuilder is a builder for instantiating TFOptions for use in
// NewTestFixture.
type TFOptionsBuilder struct {
	options *TFOptions
}

// NewTFOptionsBuilder creates a new TFOptionsBuilder with default options set.
func NewTFOptionsBuilder() *TFOptionsBuilder {
	return &TFOptionsBuilder{
		options: newTFOptions(),
	}
}

// Build returns the TFOptions as they have been configured thus far.
// No other TFOptionsBuilder functions may be called after this is called or
// a panic will be thrown.
func (b *TFOptionsBuilder) Build() *TFOptions {
	ops := b.options
	b.options = nil
	return ops
}

// DutTarget adds the dut's hostname to TFOptions.DutTargets and stores the dut
// and hint in the options for use by the fixture. Order matters here, as it
// consistent with DutIdx (i.e. the first added is the DefaultDUT).
//
// Can be included multiple times to add more duts (e.g. a companion dut).
func (b *TFOptionsBuilder) DutTarget(d *dut.DUT, h *testing.RPCHint) *TFOptionsBuilder {
	b.options.DutTargets = append(b.options.DutTargets, d.HostName())
	b.options.duts = append(b.options.duts, &dutData{dut: d, rpcHint: h})
	return b
}

// TFOption is the function signature used to modify TextFixture and adjust
// TFOptions during NewTestFixture.
type TFOption func(*TestFixture)

// PrimaryRouterTargets adds router targets to TFOptions.PrimaryRouterTargets.
func (b *TFOptionsBuilder) PrimaryRouterTargets(primaryRouterTargets ...string) *TFOptionsBuilder {
	b.options.PrimaryRouterTargets = append(b.options.PrimaryRouterTargets, primaryRouterTargets...)
	return b
}

// PcapRouterTarget sets TFOptions.PcapRouterTarget.
func (b *TFOptionsBuilder) PcapRouterTarget(pcapRouterTarget string) *TFOptionsBuilder {
	b.options.PcapRouterTarget = pcapRouterTarget
	return b
}

// AttenuatorTarget sets TFOptions.AttenuatorTarget.
func (b *TFOptionsBuilder) AttenuatorTarget(attenuatorTarget string) *TFOptionsBuilder {
	b.options.AttenuatorTarget = attenuatorTarget
	return b
}

// HostUserOverrides sets TFOptions.HostUserOverrides.
func (b *TFOptionsBuilder) HostUserOverrides(hostUserOverrides map[string]string) *TFOptionsBuilder {
	b.options.HostUserOverrides = hostUserOverrides
	return b
}

// RequirePrimaryRouter sets TFOptions.RequirePrimaryRouter.
func (b *TFOptionsBuilder) RequirePrimaryRouter(requireRouter bool) *TFOptionsBuilder {
	b.options.RequirePrimaryRouter = requireRouter
	return b
}

// EnablePacketCapture sets TFOptions.EnablePacketCapture.
func (b *TFOptionsBuilder) EnablePacketCapture(enablePacketCapture bool) *TFOptionsBuilder {
	b.options.EnablePacketCapture = enablePacketCapture
	return b
}

// EnableBridgeAndVeth sets TFOptions.EnableBridgeAndVeth.
func (b *TFOptionsBuilder) EnableBridgeAndVeth(enableBridgeAndVeth bool) *TFOptionsBuilder {
	b.options.EnableBridgeAndVeth = enableBridgeAndVeth
	return b
}

// UseFirstRouterAsPcap sets TFOptions.UseFirstRouterAsPcap.
func (b *TFOptionsBuilder) UseFirstRouterAsPcap(useFirstRouterAsPcap bool) *TFOptionsBuilder {
	b.options.UseFirstRouterAsPcap = useFirstRouterAsPcap
	return b
}

// EnableRouterReboot sets TFOptions.EnableRouterReboot.
func (b *TFOptionsBuilder) EnableRouterReboot(value bool) *TFOptionsBuilder {
	b.options.EnableRouterReboot = value
	return b
}

// EnableDutUI sets TFOptions.EnableDutUI.
func (b *TFOptionsBuilder) EnableDutUI(enableDutUI bool) *TFOptionsBuilder {
	b.options.EnableDutUI = enableDutUI
	return b
}

// EnableCellular sets TFOptions.EnableCellular.
func (b *TFOptionsBuilder) EnableCellular(enableCellular bool) *TFOptionsBuilder {
	b.options.EnableCellular = enableCellular
	return b
}

// SetDutWifiLogging sets TFOptions.EnableDutUI.
func (b *TFOptionsBuilder) SetDutWifiLogging(setLogging bool) *TFOptionsBuilder {
	b.options.SetDutWifiLogging = setLogging
	return b
}

// DutWifiLogTags sets TFOptions.DutWifiLogTags.
func (b *TFOptionsBuilder) DutWifiLogTags(dutWifiLogTags []string) *TFOptionsBuilder {
	b.options.DutWifiLogTags = dutWifiLogTags
	return b
}

// DutWifiLogLevel sets TFOptions.DutWifiLogLevel.
func (b *TFOptionsBuilder) DutWifiLogLevel(dutWifiLogLevel int) *TFOptionsBuilder {
	b.options.DutWifiLogLevel = dutWifiLogLevel
	return b
}
