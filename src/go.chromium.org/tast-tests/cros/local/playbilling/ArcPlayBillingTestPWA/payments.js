// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

const PAYMENT_METHOD = "https://play.google.com/billing";

/**
 * Attempts to make a purchase of the supplied sku.
 *
 * @param {string} sku The unit code to purchase.
 */
function buy(sku) {
    if (!window.PaymentRequest) {
        console.log("No PaymentRequest object.");
    }

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

    const request = new PaymentRequest(supportedInstruments, item_details);

    if (request.canMakePayment) {
        request.canMakePayment()
            .then(result => {
                console.log(result ?
                    "Can make payment." :
                    "Cannot make payment.");
            })
            .catch(error => console.error(error.message));
    }

    if (request.hasEnrolledInstrument) {
        request.hasEnrolledInstrument()
            .then(result => {
                console.log(result ?
                    "Has enrolled instrument." :
                    "No enrolled instrument.");

                request.show().then((response) => {
                    console.log('payment successful');
                    response.complete('success');
                });
            })
            .catch(error => console.error(error.message));
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
