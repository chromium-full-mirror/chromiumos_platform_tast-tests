// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

// this file contains fixtures that need interactions with chameleond

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.ChameleonAudioTestbed,
		Desc:            "Access Chameleond XMLRPC service",
		Contacts:        []string{"crosep-intertech@google.com"},
		Impl:            ChameleonAudioTestbedFixture{},
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
	})
}

var (
	chameleonIP = testing.RegisterVarString(
		"audio.chameleon_ip",
		"",
		"Local IP address of Chameleon (required)")
	chameleonPort = testing.RegisterVarString(
		"audio.chameleon_port",
		"9992",
		"Port for chameleond on Chameleon (optional/used)")
)

// ChameleonAudioTestbedFixture is a fixture to use chameleond XMLRPC service
// As of Feb 13 2023, chameleond is running on different hardware (v2, v3 and pi)
// Meanwhile, we have different setups including: Audiobox/USB/HDMI
// This fixture can be applied on different hardware and different setups
type ChameleonAudioTestbedFixture struct {
	Chameleond chameleon.Chameleond
}

// SetUp the ChameleonAudioTestbedFixture
func (f ChameleonAudioTestbedFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if net.ParseIP(chameleonIP.Value()) == nil {
		s.Fatal("Unable to parse Chameleon ip:", chameleonIP.Value())
	}

	chameleonURL := net.JoinHostPort(chameleonIP.Value(), chameleonPort.Value())
	chameleon, err := chameleon.NewChameleond(ctx, chameleonURL)
	if err != nil {
		s.Fatal("Unable to reach Chameleon: ", err)
	}
	f.Chameleond = chameleon
	return f
}

// TearDown the ChameleonAudioTestbedFixture
func (f ChameleonAudioTestbedFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

// Reset the ChameleonAudioTestbedFixture
func (f ChameleonAudioTestbedFixture) Reset(ctx context.Context) error {
	return f.Chameleond.Reset(ctx)
}

// PreTest the ChameleonAudioTestbedFixture
func (f ChameleonAudioTestbedFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest the ChameleonAudioTestbedFixture
func (f ChameleonAudioTestbedFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
