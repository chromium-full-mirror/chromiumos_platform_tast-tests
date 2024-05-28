// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

// FirstTpm2DataMilestone is the first milestone we have prepared for cross version with TPM2.0 simulator
var FirstTpm2DataMilestone = 88

// LatestTpm2DataMilestone is the latest milestone we have prepared for cross version with TPM2.0 simulator
var LatestTpm2DataMilestone = 125

// Tpm2DataPrefixes are the data prefixed for cross version with TPM2.0 simulator
var Tpm2DataPrefixes = map[int]string{
	88:  "R88-13597.108.0-custombuild20220717_betty_20220719",
	89:  "R89-13729.85.0-custombuild20220715_betty_20220719",
	90:  "R90-13816.106.0-custombuild20220712_betty_20220719",
	91:  "R91-13904.98.0-custombuild20220712_betty_20220719",
	92:  "R92-13982.89.0-custombuild20220712_betty_20220719",
	93:  "R93-14092.106.0-custombuild20220713_betty_20220719",
	94:  "R94-14150.592.0-custombuild20220714_betty_20220720",
	96:  "R96-14268.94.0-custombuild20220714_betty_20220719",
	97:  "R97-14324.81.0-custombuild20220715_betty_20220719",
	98:  "R98-14388.65.0-custombuild20220715_betty_20220719",
	99:  "R99-14469.76.0-custombuild20220717_betty_20220719",
	100: "R100-14526.122.0-custombuild20220718_betty_20220719",
	101: "R101-14588.134.0-custombuild20220718_betty_20220719",
	102: "R102-14695.114.0-custombuild20220718_betty_20220719",
	103: "R103-14816.99.0_betty_20220712",
	104: "R104-14909.132.0_betty_20221202",
	105: "R105-14989.107.0_betty_20221202",
	106: "R106-15054.114.0_betty_20221129",
	107: "R107-15117.112.0_betty_20221129",
	108: "R108-15183.69.0_betty_20221221",
	109: "R109-15236.82.0_novato_20230216",
	110: "R110-15278.66.0_novato_20230216",
	111: "R111-15329.61.0_novato_20230329",
	112: "R112-15359.49.0_novato_20230410",
	113: "R113-15393.65.0_novato_20230627",
	114: "R114-15437.60.0_novato_20230627",
	115: "R115-15474.84.0_novato_20230830",
	116: "R116-15509.71.0_novato_20230830",
	117: "R117-15572.63.0_novato_20231031",
	118: "R118-15604.33.0_novato_20231123",
	119: "R119-15633.69.0_amd64-generic_20231212",
	120: "R120-15662.109.0_amd64-generic_20240528",
	121: "R121-15699.70.0_amd64-generic_20240528",
	122: "R122-15753.58.0_amd64-generic_20240528",
	123: "R123-15786.60.0_amd64-generic_20240528",
	124: "R124-15823.61.0_amd64-generic_20240529",
	125: "R125-15853.52.0_amd64-generic_20240529",
}

// FirstTpmDynamicDataMilestone is the first milestone we have prepared for cross version with TPM dynamic
var FirstTpmDynamicDataMilestone = 96

// LatestTpmDynamicDataMilestone is the latest milestone we have prepared for cross version with TPM dynamic
var LatestTpmDynamicDataMilestone = 125

// TpmDynamicDataPrefixes are the data prefixed for cross version with TPM dynamic
var TpmDynamicDataPrefixes = map[int]string{
	96:  "R96-14268.94.0-custombuild20220715_reven-vmtest_20220719",
	97:  "R97-14324.81.0-custombuild20220716_reven-vmtest_20220719",
	98:  "R98-14388.65.0-custombuild20220719_reven-vmtest_20220719",
	99:  "R99-14469.76.0-custombuild20220718_reven-vmtest_20220719",
	100: "R100-14526.122.0-custombuild20220718_reven-vmtest_20220719",
	101: "R101-14588.134.0-custombuild20220718_reven-vmtest_20220719",
	102: "R102-14695.114.0-custombuild20220718_reven-vmtest_20220719",
	103: "R103-14816.99.0_reven-vmtest_20220712",
	104: "R104-14909.132.0_reven-vmtest_20221202",
	105: "R105-14989.108.0_reven-vmtest_20221202",
	106: "R106-15054.114.0_reven-vmtest_20221129",
	107: "R107-15117.112.0_reven-vmtest_20221129",
	108: "R108-15183.69.0_reven-vmtest_20221221",
	109: "R109-15236.82.0_reven-vmtest_20230216",
	110: "R110-15278.66.0_reven-vmtest_20230216",
	111: "R111-15329.61.0_reven-vmtest_20230329",
	112: "R112-15359.49.0_reven-vmtest_20230410",
	113: "R113-15393.65.0_reven-vmtest_20230627",
	114: "R114-15437.60.0_reven-vmtest_20230627",
	115: "R115-15474.84.0_reven-vmtest_20230830",
	116: "R116-15509.71.0_reven-vmtest_20230830",
	117: "R117-15572.63.0_reven-vmtest_20231031",
	118: "R118-15604.60.0_reven-vmtest_20231123",
	119: "R119-15633.69.0_reven-vmtest_20231212",
	120: "R120-15662.109.0_reven-vmtest_20240528",
	121: "R121-15699.72.0_reven-vmtest_20240528",
	122: "R122-15753.58.0_reven-vmtest_20240528",
	123: "R123-15786.60.0_reven-vmtest_20240528",
	124: "R124-15823.61.0_reven-vmtest_20240529",
	125: "R125-15853.52.0_reven-vmtest_20240529",
}

// FirstTi50DataMilestone is the first milestone we have prepared for cross version with Ti50 emulator
var FirstTi50DataMilestone = 112

// LatestTi50DataMilestone is the latest milestone we have prepared for cross version with Ti50 emulator
var LatestTi50DataMilestone = 125

// Ti50DataPrefixes are the data prefixed for cross version with Ti50 emulator
var Ti50DataPrefixes = map[int]string{
	112: "R112-15359.49.0_betty_20230410",
	113: "R113-15393.65.0_betty_20230627",
	114: "R114-15437.60.0_betty_20230627",
	115: "R115-15474.84.0_betty_20230830",
	116: "R116-15509.71.0_betty_20230830",
	117: "R117-15572.63.0_betty_20231031",
	118: "R118-15604.60.0_betty_20231123",
	119: "R119-15633.69.0_betty_20231212",
	120: "R120-15662.109.0_betty_20240528",
	121: "R121-15699.72.0_betty_20240528",
	122: "R122-15753.58.0_betty_20240528",
	123: "R123-15786.60.0_betty_20240528",
	124: "R124-15823.61.0_betty_20240528",
	125: "R125-15853.53.0_betty_20240529",
}
