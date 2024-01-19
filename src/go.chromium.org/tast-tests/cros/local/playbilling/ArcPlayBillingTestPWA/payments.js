// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

const PAYMENT_METHOD = "https://play.google.com/billing";

/**
 * Attempts to make a purchase of the supplied sku.
 *
 * @param {string} sku The unit code to purchase.
 */
async function buy(sku) {
  const supportedInstruments = [{
    supportedMethods: PAYMENT_METHOD,
    data: {
      sku: sku,
    },
  }];
  const item_details = {
    total: {
      label: "Total",
      amount: {
        currency: "AUD",
        value: "1",
      },
    },
  };

  try {
    const request = new PaymentRequest(supportedInstruments, item_details);
    const buyChecks = `canMakePayment=${
      await request.canMakePayment()}, hasEnrolledInstrument=${
      await request.hasEnrolledInstrument()}`;
    console.log(buyChecks);
    document.getElementById("buy-checks").innerHTML = buyChecks;
    const response = await request.show();
    console.log("payment successful");
    await response.complete("success");
  } catch (e) {
    document.getElementById("errors").innerHTML = e.message;
    console.error(e.message);
    throw e;
  }
}

/**
 * Attempts to get details of the supplied sku.
 *
 * @param {string} sku The unit code to purchase.
 */
async function getDetails(sku) {
  try {
    document.getElementById("errors").innerHTML = "";
    let service = await window.getDigitalGoodsService(PAYMENT_METHOD);
    let details = await service.getDetails([sku]);
    let str = JSON.stringify(details);
    document.getElementById("details").innerHTML = str;
    return str;
  } catch (e) {
    document.getElementById("errors").innerHTML = e.message;
    throw e;
  }
}
