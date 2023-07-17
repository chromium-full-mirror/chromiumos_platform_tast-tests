// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"go.chromium.org/tast/core/testing/hwdep"
)

// This list was fetched by Plx script, use "Lenovo" as primaryOemName:
// https://plx.corp.google.com/scripts2/script_61._62eca9_0000_2e5b_930b_30fd38187cc4
var lenovoModelList = []string{
	"akemi",
	"beetley",
	"blipper",
	"bookem",
	"boten",
	"botenflex",
	"fennel",
	"fennel14",
	"gooey",
	"hana",
	"homestar",
	"kinox",
	"kodama",
	"krane",
	"laser",
	"laser14",
	"liara",
	"lick",
	"lillipup",
	"lindar",
	"magneton",
	"makomo",
	"maple",
	"maple14",
	"morphius",
	"mrbland",
	"orco",
	"pantheon",
	"phaser",
	"phaser360",
	"primus",
	"pujjo",
	"pujjoflex",
	"pujjoteen",
	"pujjoteen15w",
	"pyke",
	"pyro",
	"reks",
	"robo",
	"rusty",
	"sentry",
	"steelix",
	"sycamore",
	"taeko",
	"taniks",
	"tarlo",
	"treeya360",
	"treeyac",
	"ultima",
	"vilboz",
	"vilboz14",
	"wormdingler",
}

// LenovoModels returns hardwareDeps condition with list of all Lenovo models.
func LenovoModels() hwdep.Deps {
	return hwdep.D(hwdep.Model(lenovoModelList...))
}
