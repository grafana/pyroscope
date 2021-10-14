import '@testing-library/jest-dom';
// import 'jest-canvas-mock';

const { toMatchImageSnapshot } = require('jest-image-snapshot');

expect.extend({ toMatchImageSnapshot });
/**
 * Takes a JSDOM image and returns a Node.js buffer to use
 * with jest-image-snapshot.
 * source: https://github.com/americanexpress/jest-image-snapshot/issues/279
 */
// function imageToBuffer(image: HTMLImageElement): Buffer {
//  const canvas = document.createElement('canvas');
//  canvas.width = image.width;
//  canvas.height = image.height;
//
//  const ctx = canvas.getContext('2d');
//
//  ctx.drawImage(image, 0, 0);
//
//  const base64 = canvas.toDataURL().split(',')[1];
//  return Buffer.from(base64, 'base64');
// }
//
// expect.extend({
//  toMatchImageSnapshot: function (received, options) {
//    // If these checks pass, assume we're in a JSDOM environment with the 'canvas' package.
//    if (
//      received &&
//      received.constructor &&
//      received.constructor.name === 'HTMLImageElement'
//    ) {
//      received = imageToBuffer(received);
//    }
//
//    return toMatchImageSnapshot.call(this, received, options);
//  },
// });
