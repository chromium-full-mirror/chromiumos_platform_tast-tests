// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"chromiumos/tast/testing/hwdep"
)

// NoAdvertismentFilteringHardwareSupportModels are models that do not have
// hardware support for advertisement filtering, which is needed for Fast Pair.
var NoAdvertismentFilteringHardwareSupportModels = []string{"akali", "akali360",
	"aleena", "astronaut", "asuka", "babymega", "babytiger", "bard", "barla",
	"basking", "beadrix", "blacktip", "blacktiplte", "blacktip360", "blipper",
	"blooglet", "bob", "bruce", "burnet", "careena", "caroline", "cave", "cerise",
	"chell", "cozmo", "damu", "dewatt", "dirinboz", "dru", "druwl", "dumo", "ekko",
	"electro", "elm", "epaulette", "esche", "eve", "ezkinil", "fennel", "fennel14",
	"gumboz", "hana", "hayato", "jax", "jelboz", "jelboz360", "juniper", "kakadu",
	"kappa", "karma", "kasumi", "kasumi360", "katsu", "kench", "kenzo", "kevin",
	"kodama", "krane", "landrid", "lars", "lava", "leona", "liara", "madoo",
	"nasher", "nasher360", "nautilus", "nocturne", "nuwani", "nuwani360",
	"pantheon", "pirette", "pyro", "rabbid", "robo", "robo360", "sand", "santa",
	"sentry", "shyvana", "shyvana-m", "sion", "snappy", "sona", "soraka", "stern",
	"syndra", "teemo", "treeya", "treeya360", "vayne", "vorticon", "whitetip",
	"willow"}

// FastPairHardwareDep skips all models which do not have Fast Pair hardware support.
var FastPairHardwareDep = hwdep.SkipOnModel(NoAdvertismentFilteringHardwareSupportModels...)
