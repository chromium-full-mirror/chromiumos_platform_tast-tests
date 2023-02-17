// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

class PatternChecker {
  constructor() {
    this.patternImg = null;
    this.targetImg = null;
    this.kp1 = new cv.KeyPointVector();
    this.kp2 = new cv.KeyPointVector();
    this.des1 = new cv.Mat();
    this.des2 = new cv.Mat();
    this.sift = new cv.SIFT();
    this.flann = new cv.FlannBasedMatcher();
    this.dts = new cv.Mat()
  }

  destructor() {
    if (this.patternImg) {
      this.patternImg.delete();
    }
    if (this.targetImg) {
      this.targetImg.delete();
    }
    this.kp1.delete();
    this.kp2.delete();
    this.des1.delete();
    this.des2.delete();
    this.sift.delete();
    this.flann.delete();
    this.dts.delete();
  }

  /*
   * Given a rectangle pattern with four vertice
   * (A0,B0), (A1,B1), (A2,B2), (A3,B3),
   * and its perspective transformed quadrilateral
   * with four vertice (C0,D0), (C1,D1), (C2,D2), (C3,D3),
   * for i in range(0,4) check
   * all Ai*(1-H_BOUND_COEF0) < Ci < Ai*(1+H_BOUND_COEF1)
   * all Bi*(1-V_BOUND_COEF0) < Di < Bi*(1+V_BOUND_COEF1)
   * Checks the |aspectRatio| camera FOV of |facing| camera is aligned with
   * pattern.
   * @param {!ImageData} pattern
   * @param {!ImageData} imageData
   * @param {!Canvas} canvasDebug
   * @param {number} proportion
   * @return {boolean}
   * @private
   */
  checkAlign(pattern, imageData, canvasDebug = null, proportion = 1) {
    const H_BOUND_COEF0 = 0.1;
    const H_BOUND_COEF1 = 0.30;
    const V_BOUND_COEF0 = 0.1;
    const V_BOUND_COEF1 = 0.30;
    const MIN_MATCH_COUNT = 20;
    const matches = new cv.DMatchVectorVector();
    const goodMatches = new cv.DMatchVector();
    this.patternImg = cv.imread(pattern);
    this.targetImg = cv.matFromImageData(imageData);

    // Find out keypoints and descriptor of pattern and target image by SIFT.
    this.sift.detectAndCompute(
        this.patternImg, new cv.Mat(), this.kp1, this.des1);
    this.sift.detectAndCompute(
        this.targetImg, new cv.Mat(), this.kp2, this.des2);

    /*
     * Match descriptor vectors with a FLANN based matcher.
     * The less the distance between two descriptors,
     * the more they are similar.
     */
    try {
      this.flann.knnMatch(this.des1, this.des2, matches, 2);
    } catch (e) {
      console.warn('knnMatch error:', e);
      return false;
    }
    for (let i = 0; i < matches.size(); ++i) {
      const match = matches.get(i);
      const dMatch1 = match.get(0);
      const dMatch2 = match.get(1);
      if (dMatch1.distance < dMatch2.distance * 2) {
        goodMatches.push_back(dMatch1);
      }
    }

    /*
     * If the size of matched keypoints set is too small,
     * consider it as not match.
     */
    if (goodMatches.size() < MIN_MATCH_COUNT) {
      matches.delete();
      goodMatches.delete();
      return false;
    }

    /*
     * Given a set of keypoints from pattern image (points1)
     * and a set of keypoints from target image (points2),
     * find out the vertices of the pattern after perspective transformation.
     */
    const points1 = [];
    const points2 = [];
    for (let i = 0; i < goodMatches.size(); i++) {
      points1.push(this.kp1.get(goodMatches.get(i).queryIdx).pt.x);
      points2.push(this.kp2.get(goodMatches.get(i).trainIdx).pt.x);
      points1.push(this.kp1.get(goodMatches.get(i).queryIdx).pt.y);
      points2.push(this.kp2.get(goodMatches.get(i).trainIdx).pt.y);
    }

    /*
     * Call findHomography to get a perspective transform
     * matrix from points1 to points2
     */
    const mat1 = cv.matFromArray(points1.length / 2, 1, cv.CV_64FC2, points1);
    const mat2 = cv.matFromArray(points2.length / 2, 1, cv.CV_64FC2, points2);
    const h = cv.findHomography(mat1, mat2, cv.RANSAC, 5.0);
    let width = this.patternImg.size().width;
    let height = this.patternImg.size().height;
    mat1.delete();
    mat2.delete();
    let result = false;
    if (!h.empty()) {
      const ptsTmp = [0, 0, 0, height - 1, width - 1, height - 1, width - 1, 0];
      const pts = cv.matFromArray(4, 1, cv.CV_32FC2, ptsTmp);
      const MV = new cv.Mat(4, 1, cv.CV_32SC2);
      /*
       * Use the perspective transform matrix to find out the perspective
       * transformed object.
       */
      cv.perspectiveTransform(pts, this.dts, h)
      pts.delete();
      this.dts.convertTo(MV, cv.CV_32SC2);
      const p = [
        [this.dts.floatPtr(0, 0)[0], this.dts.floatPtr(0, 0)[1]],
        [this.dts.floatPtr(1, 0)[0], this.dts.floatPtr(1, 0)[1]],
        [this.dts.floatPtr(2, 0)[0], this.dts.floatPtr(2, 0)[1]],
        [this.dts.floatPtr(3, 0)[0], this.dts.floatPtr(3, 0)[1]],
      ]
      width = this.targetImg.size().width,
      height = this.targetImg.size().height;
      const hBound = [
        width * H_BOUND_COEF0 * -1, width * H_BOUND_COEF1,
        width * (1 - H_BOUND_COEF1), width * (1 + H_BOUND_COEF0)
      ];
      const vBound = [
        height * V_BOUND_COEF0 * -1, height * V_BOUND_COEF1,
        height * (1 - V_BOUND_COEF1), height * (1 + V_BOUND_COEF0)
      ];
      const bound = [
        [hBound[0], hBound[1], vBound[0], vBound[1]],
        [hBound[0], hBound[1], vBound[2], vBound[3]],
        [hBound[2], hBound[3], vBound[2], vBound[3]],
        [hBound[2], hBound[3], vBound[0], vBound[1]]
      ];

      // Draw the matched result for debug.
      if (canvasDebug) {
        const imMatches = new cv.Mat();
        let markersVector = new cv.MatVector();
        markersVector.push_back(MV);
        // Draw the boundary with white line.
        cv.polylines(
            this.targetImg, markersVector, true, new cv.Scalar(255, 0, 0), 3);
        markersVector.delete();
        markersVector = new cv.MatVector();
        let area = new cv.matFromArray(4, 1, cv.CV_32SC2, [
          hBound[0], vBound[0], hBound[0], vBound[1], hBound[1], vBound[1],
          hBound[1], vBound[0]
        ]);
        markersVector.push_back(area);
        area = new cv.matFromArray(4, 1, cv.CV_32SC2, [
          hBound[2], vBound[0], hBound[2], vBound[1], hBound[3], vBound[1],
          hBound[3], vBound[0]
        ]);
        markersVector.push_back(area);
        area = new cv.matFromArray(4, 1, cv.CV_32SC2, [
          hBound[2], vBound[2], hBound[2], vBound[3], hBound[3], vBound[3],
          hBound[3], vBound[2]
        ]);
        markersVector.push_back(area);
        area = new cv.matFromArray(4, 1, cv.CV_32SC2, [
          hBound[0], vBound[2], hBound[0], vBound[3], hBound[1], vBound[3],
          hBound[1], vBound[2]
        ]);
        markersVector.push_back(area);
        // Draw the legal area.
        cv.polylines(
            this.targetImg, markersVector, true, [255, 0, 0, 255], 2, 4);
        // Draw keypoint matches with green line.
        cv.drawMatches(
            this.patternImg, this.kp1, this.targetImg, this.kp2, goodMatches,
            imMatches, new cv.Scalar(0, 255, 0, 255));
        const dsize = new cv.Size(
            Math.ceil(imMatches.size().width * proportion),
            Math.ceil(imMatches.size().height * proportion));
        cv.resize(imMatches, imMatches, dsize, 0, 0, cv.INTER_AREA);
        cv.imshow(canvasDebug, imMatches);
        area.delete();
        markersVector.delete();
        imMatches.delete();
      }
      MV.delete();

      // Check whether all the 4 vertices lays in the correct area or not.
      let normal = true;
      for (let i = 0; i < 4; i++) {
        const x = p[i][0];
        const y = p[i][1];
        if (x < bound[i][0] || x > bound[i][1] || y < bound[i][2] ||
            y > bound[i][3]) {
          normal = false;
        }
      }

      /*
      /* Check whether all the 4 vertices lays in the correct area or not
       * in the rotated case.
       */
      let rotate = true;
      for (let i = 0; i < 4; i++) {
        const x = p[(i + 2) % 4][0];
        const y = p[(i + 2) % 4][1];
        if (x < bound[i][0] || x > bound[i][1] || y < bound[i][2] ||
            y > bound[i][3]) {
          rotate = false;
        }
      }
      result = normal || rotate;
    }
    h.delete();
    matches.delete();
    goodMatches.delete();
    return result;
  }
}