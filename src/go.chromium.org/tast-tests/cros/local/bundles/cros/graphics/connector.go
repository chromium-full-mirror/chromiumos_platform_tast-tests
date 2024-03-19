// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/graph"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// dpPlusModels are models that have DP++ support in BIOS.
// This list contains models that have wrong bios setting as of 2020/10.
// Please contact chromeos-gfx@ team to edit the list if new device exposes dp++ support on purpose.
var dpPlusModels = []string{
	"asuka",
	// brask family
	"aurash", "brask", "kinox", "kuldax", "moli",
	"cave",
	"caroline",
	"chell",
	// drallion family
	"drallion", "drallion360",
	// fizz family
	"fizz", "jax", "kench", "sion", "teemo",
	"guado",
	"karma",
	"lars",
	"lili",
	// nautilus family
	"nautilus", "nautiluslte",
	"nocturne",
	// nami family
	"akali", "akali360", "bard", "ekko", "pantheon", "sona", "syndra", "vayne",
	// octopus family
	"dorp",
	"garfour",
	// puff family
	"duffy", "faffy", "kaisa", "noibat", "puff", "wyvern",
	// rammus family
	"leona", "shyvana",
	"rikku",
	"sentry",
	"soraka",
	// sarien family
	"arcada", "sarien",
	"tidus",
	// volteer family
	// TODO(b:190100059): Remove once VBT is fixed.
	"volteer2",
	// reven family
	"reven",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Connector,
		Desc:         "Checks the validity of display connector configurations",
		BugComponent: "b:188154", // ChromeOS > Platform > Graphics > Display
		Contacts: []string{
			"chromeos-gfx@google.com",
			"pwang@chromium.org",
		},
		SoftwareDeps: []string{"no_qemu"},
		Fixture:      "gpuWatchHangs",
		Params: []testing.Param{
			{
				Name:              "",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(dpPlusModels...)),
				ExtraAttr:         []string{"group:mainline", "group:cq-medium"},
			}, {
				Name:              "bad_bios",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(dpPlusModels...)),
				ExtraAttr:         []string{"group:graphics", "graphics_nightly"},
			},
		},
	})
}

// Connector checks various attributes of the connectors settings via modetest.
func Connector(ctx context.Context, s *testing.State) {
	connectors, err := graphics.ModetestConnectors(ctx)
	if err != nil {
		s.Fatal("Failed to get connectors: ", err)
	}
	defer graphics.DumpModetestOnError(ctx, "-c", s.OutDir(), s.HasError)

	if err := checkUniqueEncoders(ctx, connectors); err != nil {
		s.Error("Failed to have check unique encoders: ", err)
	}
}

// checkUniqueEncoders checks if every connector can be assigned a unique encoder concurrently.
func checkUniqueEncoders(ctx context.Context, connectors []*graphics.Connector) error {
	g := graph.NewBipartite()
	for _, connector := range connectors {
		for _, encoderID := range connector.Encoders {
			g.AddEdge(int(connector.ConnectorID), int(encoderID))
		}
	}

	maxMatch := g.MaxMatching()
	if maxMatch != len(connectors) {
		return errors.Errorf(
			"not all connector has a unqiue encoder matching (expect %d but got %d)",
			len(connectors), maxMatch)
	}
	return nil
}
