// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dep

import (
	"go.chromium.org/tast/core/testing/hwdep"
)

// This list was fetched by Plx script, use "Acer" as primaryOemName:
// https://plx.corp.google.com/scripts2/script_61._62eca9_0000_2e5b_930b_30fd38187cc4
var acerModelList = []string{
	"akali360",
	"aleena",
	"banon",
	"bard",
	"blorb",
	"blue",
	"bobba",
	"bobba360",
	"buddy",
	"cozmo",
	"craask",
	"craaskana",
	"craaskbowl",
	"craaskino",
	"craaskov",
	"craaskvin",
	"craasneto",
	"craaswell",
	"cyan",
	"dewatt",
	"dita",
	"dochi",
	"droid",
	"dru",
	"edgar",
	"ekko",
	"electro",
	"elm",
	"ezkinil",
	"gik",
	"gnawtyplus",
	"juniper",
	"kaisa",
	"kano",
	"karis",
	"karma",
	"kasumi",
	"kenzo",
	"kindred",
	"kled",
	"lalala",
	"lars",
	"lazor",
	"lili",
	"limozeen",
	"magister",
	"maglet",
	"maglia",
	"maglith",
	"magma",
	"magneto",
	"magolor",
	"magpie",
	"markarth",
	"moli",
	"omnigul",
	"omniknight",
	"osiris",
	"pico",
	"pico6",
	"quackingstick",
	"sand",
	"sion",
	"sparky",
	"sparky360",
	"squirtle",
	"tifa",
	"tomato",
	"voema",
	"volet",
	"volmar",
	"volta",
	"voltorb",
	"voxel",
	"willow",
	"zavala",
}

// AcerModels returns hardwareDeps condition with list of all Asus models.
func AcerModels() hwdep.Deps {
	return hwdep.D(hwdep.Model(acerModelList...))
}
