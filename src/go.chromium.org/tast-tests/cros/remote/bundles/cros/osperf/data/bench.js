// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

function sleep(time) {
  return new Promise((resolve) => {setTimeout(resolve, time)})
}

function getInnerTextOfTab(tabId) {
  return new Promise((resolve) => {
    chrome.scripting.executeScript(
        {
          func: () => {
            return document.body.innerText;
          },
          target: {
            tabId: tabId,
          }
        },
        (injectionResults) => {
          for (const frameResult of injectionResults) {
            const result = frameResult.result;
            resolve(result);
          }
        });
  });
}

async function getInnerTextOfUrl(url) {
  const tab = await chrome.tabs.create({url: url, active: false});
  const text = await getInnerTextOfTab(tab.id);
  await chrome.tabs.remove(tab.id);
  return text;
}

function extractBiosInfoAttr(biosInfoLines, key) {
  return biosInfoLines.filter((s) => s.startsWith(key))[0]
      .split(' = ')[1]
      .split('#')[0]
      .trim();
}

const runCycle = async function(numTabs) {
  // returns: tab open latencies: [Number; numTabs]
  const result = [];
  const tabIdToBeRemovedList = [];
  for (let i = 0; i < numTabs; i++) {
    const t0 = performance.now();
    // Open a tab with a data URL, to make sure that it gets an unique security
    // context (which means the tab is created in a new process.)
    // c.f.
    // https://developer.mozilla.org/en-US/docs/Web/Security/Same-origin_policy#inherited_origins
    const tid = (await chrome.tabs.create({
                  url: 'data:,Hello%2C%20World%21',
                  active: false
                })).id;
    while (true) {
      const t = await chrome.tabs.get(tid);
      if (t.status === 'complete') {
        tabIdToBeRemovedList.push(tid);
        break;
      }
    }
    const t1 = performance.now();
    const diff = t1 - t0;
    result.push(diff);
  }
  for (const tabId of tabIdToBeRemovedList) {
    do {
      try {
        await chrome.tabs.remove(tabId);
      } catch {
        console.log('Remove failed. Retrying.')
        continue;
      }
    } while (0)
  }
  return result;
};

const runBench = async function(numCycles, numTabs) {
  // returns: latencies: [[Number; iterCount]; numCycles]
  const result = [];
  for (let i = 0; i < numCycles; i++) {
    result.push(await runCycle(numTabs));
  }
  return result;
};

const log = (div, s) => {
  div.innerText += s;
  div.innerText += '\n';
};

document.addEventListener('DOMContentLoaded', function() {
  const takeLogButton = document.getElementById('takeLogButton');
  const benchButton = document.getElementById('benchButton');
  const copyResultButton = document.getElementById('copyResultButton');
  const shortResultPre = document.getElementById('shortResultPre');
  const benchResultPre = document.getElementById('benchResultPre');
  const histogramStepWidth = 10;
  const histogramNumSteps = 30;
  const totalHistogram = [];
  const totalHistogramXList = [];
  let runCount = 0;
  for (let i = 0; i < histogramNumSteps; i++) {
    totalHistogram[i] = 0;
    totalHistogramXList[i] = i * histogramStepWidth;
  }
  const startBench = async () => {
    const numTabsInput = document.getElementById('numTabsInput');
    const numTabs = parseInt(numTabsInput.value);

    const numCyclesInput = document.getElementById('numCyclesInput');
    const numCycles = parseInt(numCyclesInput.value);

    const numConvergedResultsInput =
        document.getElementById('numConvergedResultsInput');
    const numConvergedResults = parseInt(numConvergedResultsInput.value);

    const result = [];
    let iterCount = 0;
    const convergedMeans = [];
    const runBenchAndProcess = async () => {
      // Returns: isConverged
      iterCount++;
      const r = await runBench(numCycles, numTabs);
      result.push(r);

      let resultStatus = 'not_enough_results_yet';
      let t1 = '';
      let t2 = '';
      let t3 = '';
      let meanAll = '';
      if (result.length >= 3) {
        let x1 = result[result.length - 1].flat();
        let x2 = result[result.length - 2].flat();
        let x3 = result[result.length - 3].flat();
        t1 = ttest(x1, x2);
        t2 = ttest(x2, x3);
        t3 = ttest(x3, x1);
        if (t1 < pValueLimit && t2 < pValueLimit && t3 < pValueLimit) {
          meanAll = mean([mean(x1), mean(x2), mean(x3)]);
          resultStatus = 'converged';
          convergedMeans.push(meanAll);
        } else {
          resultStatus = 'not_converged_yet';
        }
      }
      log(benchResultPre,
          `${(new Date()).toISOString()},${iterCount},${resultStatus},${
              meanAll},${t1},${t2},${t3},${r}`);
      return (resultStatus === 'converged');
    };
    const t0 = performance.now();
    await runBenchAndProcess();
    await runBenchAndProcess();
    const pValueLimit = 1;
    let totalRunCount = 2;
    for (let i = 0; i < numConvergedResults; i++) {
      while (true) {
        totalRunCount++;
        if (await runBenchAndProcess()) {
          break;
        }
      }
    }
    const t1 = performance.now();
    const totalDuration = t1 - t0;
    const resultMean = mean(convergedMeans);
    const resultMax = Math.max(...convergedMeans);
    const resultMin = Math.min(...convergedMeans);
    log(shortResultPre,
        `${(new Date()).toISOString()},${resultMean},${resultMax},${
            resultMin},${convergedMeans.length},${totalRunCount},${
            totalDuration}`);
    const info = {
      tab_open_latency_mean: resultMean,
      tab_open_latency_max: resultMax,
      tab_open_latency_min: resultMin,
      total_duration: totalDuration,
      total_run_count: totalRunCount,
      converged_means: convergedMeans,
      result_log: benchResultPre.innerText,
    };
    console.log(info);
    return info;
  };
  window.startBench = startBench;
  benchButton.addEventListener('click', startBench);
  async function takeLog(benchResultPre) {
    const biosInfo = await getInnerTextOfUrl('file:///var/log/bios_info.txt');
    const biosInfoLines = biosInfo.split('\n');
    const hwid = extractBiosInfoAttr(biosInfoLines, 'hwid');
    const fwid = extractBiosInfoAttr(biosInfoLines, 'fwid');
    log(benchResultPre, `0,hwid,${hwid}`);
    log(benchResultPre, `0,fwid,${fwid}`);
  }
  takeLogButton.addEventListener('click', async () => {
    await takeLog(benchResultPre);
  });
  copyResultButton.addEventListener('click', async () => {
    navigator.clipboard.writeText(benchResultPre.innerText)
        .then(
            () => {
              const old = copyResultButton.innerText;
              copyResultButton.innerText = 'OK!';
              setTimeout(() => {
                copyResultButton.innerText = old;
              }, 1000);
            },
            () => {
              const old = copyResultButton.innerText;
              copyResultButton.innerText = 'Failed...';
              setTimeout(() => {
                copyResultButton.innerText = old;
              }, 1000);
            },
        );
  });
});

function mean(x) {
  let sum = 0;
  for (const v of x) sum += v;
  return sum / x.length;
}
function variance(x) {
  const m = mean(x);
  let sum = 0;
  for (const v of x) sum += (v - m) * (v - m);
  return sum / x.length;
}
function ttest(x1, x2) {
  let mean1 = mean(x1);
  let mean2 = mean(x2);
  let v1 = variance(x1);
  let v2 = variance(x2);
  return (Math.abs(mean1 - mean2) / Math.sqrt(v1 / x1.length + v2 / x2.length));
}
