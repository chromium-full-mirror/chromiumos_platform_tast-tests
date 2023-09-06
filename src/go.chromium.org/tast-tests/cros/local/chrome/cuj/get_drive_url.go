// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"

	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/errors"
)

// GetTestDocURL returns a Google Doc link according to the following:
// 1. The link returned is the same for all devices with the same model.
// 2. The link is random across all models.
//
// The link is a copy of the following document:
// https://docs.google.com/document/d/1MW7lAk9RZ-6zxpObNwF0r80nu-N1sXo5f7ORG4usrJQ/edit
func GetTestDocURL(ctx context.Context) (string, error) {
	return getDriveURL(ctx, "document", []string{
		"1uumjjXN0cqHxApli0Vzd-vc8o7PiajdeSfJDmG8jpQs",
		"1gVBEG2Kol2fMW0JQlqRFWXGCm38-QCZx3yvygOhYqK4",
		"1zVpiJj0GA2YY6LOtS5rXf4EjTReXhd691TbA2r5ZnsY",
		"13WzDOncJ5I87tl3Zy40YWjBXjFH_7TAooArybvihOtc",
		"1JMBY6bq4mkOOu89ZGFTzYuCcFpLF-feGiElrb4tXv4A",
		"1v-eN9l898RLiEjBHOVaVmaqRbiGiqMchctLTrQth4mQ",
		"1dLZza0yyX4TiyOBZbpp6zJydrk-_kd4sOydRUW4BDN4",
		"1cecnr4xcylpcMUS9NVgvcyfZbqya89GEtDHTpMbf5Uc",
		"1kUHa73haiMM8Cl4EQs0sB5Lpavtrm0Edp4Sbh2-iZc0",
		"1xGLLBWTg3y_Ypnb9c6Z2PRMj0957oz8dIOjRHBJCPH4",
	})
}

// GetTestSlidesURL is similar to GetTestDocURL, but it returns a
// Google Slides link.
//
// The link is a copy of the following document:
// https://docs.google.com/presentation/d/1lItrhkgBqXF_bsP-tOqbjcbBFa86--m3DT5cLxegR2k/edit
func GetTestSlidesURL(ctx context.Context) (string, error) {
	return getDriveURL(ctx, "presentation", []string{
		"11-mh-vzb-ZEocqoafgU-KA1F1eql-p1K5kHYpbcLf3k",
		"1Jx_JHIWcrBSAF_PIrcFISQreGhprRm2gveQixOZEQio",
		"1yPZ0b6FpDTyUY92XjDQhC1KVmPw5U0mC1a0cY31-BuU",
		"1r7eOX9X3HOZRA_8s--Ay-kfKifZlmAGuZX6iI7qjReU",
		"1N4dKIoMKZBYVgdIRcp3fE-NH1g9nnMRbySKxs56ctX8",
		"1uev_ZfU6PgjPFMUCZOIJr0rxBdswzSG-8GYOJe6i90Y",
		"1P1fIp81hhVbWd5ZRw5v2U1eii3pebhTyXl2DQijbSBo",
		"16vRzSKAwx_QFV75H1t3GUrlq5EPit3Y5yNx_yWLZ3j8",
		"1-aSSvvDdnugUUfygj-opVUltSpZwJLae4IqiGTiMRc8",
		"1LmLydzA4tSYB6A1czpO1sUCWlMZ7QSp6CgtGOKRptBI",
	})
}

// GetTestSheetsURL is similar to GetTestDocURL, but it returns a
// Google Sheets link.
//
// The link is a copy of the following document:
// https://docs.google.com/spreadsheets/d/1I9jmmdWkBaH6Bdltc2j5KVSyrJYNAhwBqMmvTdmVOgM/edit
func GetTestSheetsURL(ctx context.Context) (string, error) {
	return getDriveURL(ctx, "spreadsheets", []string{
		"1Ij1sou7HIcydmLPR-s3xSe-cmap2H7lFhCErHX-HBmU",
		"1sKoKExkl_mpLF-HVz34Y0lpEIvaVFZzhB3FUpkJqY80",
		"1SVgMhg48r6LoZgXDbyNeG8_ZnVd1ZboYSMN2rWtQJ6c",
		"1IeiKPacIP4j6Wi1VbAyPGftEZlnr1hVNZRGwI8cjoyk",
		"16zIJDAn4zhKGH1sPKky7i6pXD7_yJSjzwbmG4jWYAJ4",
		"1TKUNSOINXg1mMM0_XkQ4oFXAd3CD--O_YyUv93U_Kio",
		"1hotZ9nrPEDzaBTzEfEayX5QwvGxarXoxNMG7x3CXzlM",
		"1r4Y1ASgTe-4-TBF93fXKMG3BR6hHBuKqi6FjFV9FeLw",
		"1OpqvOCdW-bQZTv_XXbb96UNAnFFN4P242Ae-pF1OE70",
		"1U9EJ9TXWnDH2CsBjR0t0jXpj2JK7HDcX9kE7J6V0vEY",
	})
}

// GetTestSheetsViewerURL is similar to GetTestSheetsURL, but it returns a
// Google Sheets link in viewer mode.
//
// The link is a copy of the following document:
// https://docs.google.com/spreadsheets/d/1I9jmmdWkBaH6Bdltc2j5KVSyrJYNAhwBqMmvTdmVOgM/edit
func GetTestSheetsViewerURL(ctx context.Context) (string, error) {
	return getDriveURL(ctx, "spreadsheets", []string{
		"1GpCahLvVDdx3UPLBxnHecr1RbrWHsSteJOIU-0sasWM",
		"1K-0dQd_aoIF168FdKawdjnRkmtUmGUv0v-bZNYv8xTU",
		"1p7oxRkh1ikJvhHyMmCuLQqUAuCNlCE0jAX1Il85E5mk",
		"13O2iHzUP5XiJdGjMd6tNY_xT7i-NB1DGt2GGBcC52X8",
		"1lJ1aRHRnlhWKonjjs-LQv0XAXzlcyv_K_RpLJzhohNA",
		"1y6u5kcyOsBiHpOalOHniryxoy3-_YOvbspsJ-AetKCo",
		"1u04HJRRzYOgcc-OndU2oCou0tCqNyrhfQqrx78pFsTA",
		"1OoTFOGkIEXZeOznBi6GJIl1VjviaFXqNlG6gqvgAEZQ",
		"1Rw3InRXK33slA3XV1NicPv-BiNEK51pzVrrCTJflX8w",
		"1Z-yd3k2VJZnqGT_JQ3fJCA2zVYXdERFcDbnqAVoYJPk",
	})
}

// getDriveURL constructs a drive link in the following format:
// https://docs.google.com/<|fileType|>/d/<random id from |ids|>/edit
// The "random id from ids" is deterministic for each model, and random
// across all models. For example, the same model should open up the
// same link each time that it runs the test, but each model should
// have an equal chance of opening up any of the available links.
func getDriveURL(ctx context.Context, fileType string, ids []string) (string, error) {
	model, err := crosconfig.Get(ctx, "/", "name")
	if err != nil {
		return "", errors.Wrap(err, "could not find model name")
	}

	// Seed the random number generator with the model name, to ensure
	// each model gets the same document on each run.
	hash := fnv.New64a()
	hash.Write([]byte(model))
	rand.Seed(int64(hash.Sum64()))

	return fmt.Sprintf("https://docs.google.com/%s/d/%s/edit", fileType, ids[rand.Intn(len(ids))]), nil
}

// GetTestDocCommentURL is like GetTestDocURL, but returns a link to a
// comment near the bottom of the document. This is useful for loading
// the entire document by scrolling to the bottom (or close enough).
func GetTestDocCommentURL(ctx context.Context) (string, error) {
	doc, err := GetTestDocURL(ctx)
	if err != nil {
		return "", err
	}
	return doc + "?disco=AAAAP6EbSF8", nil
}
