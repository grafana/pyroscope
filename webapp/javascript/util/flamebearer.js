// ISC License

// Copyright (c) 2018, Mapbox

// Permission to use, copy, modify, and/or distribute this software for any purpose
// with or without fee is hereby granted, provided that the above copyright notice
// and this permission notice appear in all copies.

// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
// REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
// FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
// INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
// OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
// TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
// THIS SOFTWARE.

// This component is based on flamebearer project
//   https://github.com/mapbox/flamebearer

function deltaDiff(levels, start, step) {
  for (const level of levels) {
    let prev = 0;
    for (let i = start; i < level.length; i += step) {
      level[i] += prev;
      prev = level[i] + level[i + 1];
    }
  }
}

export function deltaDiffWrapper(format, levels) {
  if (format === "double") {
    deltaDiff(levels, 0, 7);
    deltaDiff(levels, 3, 7);

  } else {
    deltaDiff(levels, 0, 4);
  }
}

// format=single
//   j = 0: x start of bar
//   j = 1: width of bar
//   j = 3: position in the main index (jStep)
//
// format=double
//   j = 0,3: x start of bar =>     x = (level[0] + level[3]) / 2
//   j = 1,4: width of bar   => width = (level[4] + level[1]) / 2
//                           =>  diff = (level[4] - level[1]) / (level[1] + level[4])
//   j = 6  : position in the main index (jStep)

const formatSingle = {
  format: "single",
  jStep : 4,
  jName : 3,
  getBarOffset:    (level, j) => level[j],
  getBarTotal:     (level, j) => level[j + 1],
  getBarTotalDiff: (level, j) => 0,
  getBarSelf:      (level, j) => level[j + 2],
  getBarSelfDiff:  (level, j) => 0,
  getBarName:      (level, j) => level[j + 3],
}

const formatDouble = {
  format: "double",
  jStep : 7,
  jName : 6,
  getBarOffset:    (level, j) => (level[j]     + level[j + 3]),
  getBarTotal:     (level, j) => (level[j + 4] + level[j + 1]),
  getBarTotalLeft: (level, j) =>  level[j + 1],
  getBarTotalRght: (level, j) =>  level[j + 4],
  getBarTotalDiff: (level, j) => (level[j + 4] - level[j + 1]),
  getBarSelf:      (level, j) => (level[j + 5] + level[j + 2]),
  getBarSelfLeft:  (level, j) =>  level[j + 2],
  getBarSelfRght:  (level, j) =>  level[j + 5],
  getBarSelfDiff:  (level, j) => (level[j + 5] - level[j + 2]),
  getBarName:      (level, j) =>  level[j + 6],
}

export function parseFlamebearerFormat(format) {
  const isSingle = format !== "double";
  if (isSingle) return formatSingle;
  else return formatDouble;
}
