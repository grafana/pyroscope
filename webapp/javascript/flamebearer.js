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

import murmurhash3_32_gc from './murmur3';
import {numberWithCommas} from './util/format';

export function deltaDiff(levels) {
  for (const level of levels) {
    let prev = 0;
    for (let i = 0; i < level.length; i += 3) {
      level[i] += prev;
      prev = level[i] + level[i + 1];
    }
  }
}

export default function render(flamebearerData) {
  const introEl = document.getElementById('intro');
  const searchEl = document.getElementById('search');
  const highlightEl = document.getElementById('highlight');
  const tooltipEl = document.getElementById('tooltip');
  const canvas = document.getElementById('flamegraph-canvas');
  const resetBtn = document.getElementById('reset');
  const ctx = canvas.getContext('2d');

  let { names, levels, numTicks } = flamebearerData;

  let rangeMin = 0;
  let rangeMax = 1;
  let topLevel = 0;
  let selectedLevel = 0;
  let query = '';
  let graphWidth, pxPerTick;

  const pxPerLevel = 18;
  const collapseThreshold = 5;
  const hideThreshold = 0.5;
  const labelThreshold = 20;

  highlightEl.style.height = pxPerLevel + 'px';

  render();

  //   window.onhashchange = () => {
  //     //   updateZoom();
  //       render();
  //   };
  canvas.onclick = (e) => {
    const { i, j } = xyToBar(e.offsetX, e.offsetY);
    if (j === -1) return;

    updateZoom(i, j);
    render();
    removeHover();
  };
  resetBtn.onclick = () => {
    searchEl.value = query = '';
    updateZoom(0, 0);
    render();
  };
  window.onresize = render;

  searchEl.oninput = (e) => {
    query = e.target.value;
    render();
  };

  function updateZoom(i, j) {
    if (!isNaN(i) && !isNaN(j)) {
      selectedLevel = i;
      topLevel = 0;
      rangeMin = levels[i][j] / numTicks;
      rangeMax = (levels[i][j] + levels[i][j + 1]) / numTicks;
    } else {
      selectedLevel = 0;
      topLevel = 0;
      rangeMin = 0;
      rangeMax = 1;
    }
  }

  function tickToX(i) {
    return (i - numTicks * rangeMin) * pxPerTick;
  }

  function render() {
    if (!levels) return;


   resetBtn.style.visibility = selectedLevel === 0 ? 'hidden' : 'visible';


    graphWidth = canvas.width = canvas.clientWidth;
    canvas.height = pxPerLevel * (levels.length - topLevel);
    canvas.style.height = canvas.height + 'px';

    if (devicePixelRatio > 1) {
      canvas.width *= 2;
      canvas.height *= 2;
      ctx.scale(2, 2);
    }

    pxPerTick = graphWidth / numTicks / (rangeMax - rangeMin);

    ctx.textBaseline = 'middle';
    ctx.font = '300 12px system-ui, -apple-system, "Segoe UI", "Roboto", "Ubuntu", "Cantarell", "Noto Sans", sans-serif, "Apple Color Emoji", "Segoe UI Emoji", "Segoe UI Symbol", "Noto Color Emoji"';

    for (let i = 0; i < levels.length - topLevel; i++) {
      const level = levels[topLevel + i];

      for (let j = 0; j < level.length; j += 3) {
        const barIndex = level[j];
        const x = tickToX(barIndex);
        const y = i * pxPerLevel;
        let numBarTicks = level[j + 1];

        const inQuery = query && (names[level[j + 2]].indexOf(query) >= 0) || false;

        // merge very small blocks into big "collapsed" ones for performance
        const collapsed = numBarTicks * pxPerTick <= collapseThreshold;
        // const collapsed = false;
        if (collapsed) {
            while (
                j < level.length - 3 &&
                barIndex + numBarTicks === level[j + 3] &&
                level[j + 4] * pxPerTick <= collapseThreshold &&
                (inQuery === (query && (names[level[j + 5]].indexOf(query) >= 0) || false))
            ) {
                j += 3;
                numBarTicks += level[j + 1];
            }
        }

        const sw = numBarTicks * pxPerTick - (collapsed ? 0 : 0.5);
        const sh = pxPerLevel - 0.5;

        //   if (x < -1 || x + sw > graphWidth + 1 || sw < hideThreshold) continue;

        ctx.beginPath();
        roundRect(ctx, x, y, sw, sh, 3);

        const ratio = numBarTicks / numTicks;

        const a = selectedLevel > i ? 0.33 : 1;
        if (!collapsed) {
          ctx.fillStyle = inQuery ? 'lightgreen' : colorBasedOnName(names[level[j + 2]], a);
        } else {
          ctx.fillStyle = inQuery ? 'lightgreen' : colorGreyscale(200, 0.66);
        }
        ctx.fill();

        if (!collapsed && sw >= labelThreshold) {

          const percent = Math.round(10000 * ratio) / 100;
          const name = `${names[level[j + 2]]} (${percent}%, ${numberWithCommas(numBarTicks)} samples)`;

          ctx.save();
          ctx.clip();
          ctx.fillStyle = 'black';
          ctx.fillText(name, Math.max(x, 0) + 3, y + sh / 2);
          ctx.restore();
        }
      }
    }
  }

  function colorBasedOnName(name, a){
    const rand = murmurhash3_32_gc(name);
    const r = Math.round(205 + rand % 50);
    const g = Math.round(100 + rand % 70);
    const b = Math.round(0 + rand % 55);
    return `rgba(${r}, ${g}, ${b}, ${a})`;
  }

  function colorGreyscale(v, a){
    return `rgba(${v}, ${v}, ${v}, ${a})`;
  }

  // pixel coordinates to bar coordinates in the levels array
  function xyToBar(x, y) {
    const i = Math.floor(y / pxPerLevel) + topLevel;
    const j = binarySearchLevel(x, levels[i]);
    return { i, j };
  }

  // binary search of a block in a stack level
  function binarySearchLevel(x, level) {
    let i = 0;
    let j = level.length - 3;
    while (i <= j) {
      const m = 3 * ((i / 3 + j / 3) >> 1);
      const x0 = tickToX(level[m]);
      const x1 = tickToX(level[m] + level[m + 1]);
      if (x0 <= x && x1 >= x) {
        return x1 - x0 > collapseThreshold ? m : -1;
      }
      if (x0 > x) {
        j = m - 3;
      } else {
        i = m + 3;
      }
    }
    return -1;
  }

  if (window.orientation === undefined) {
    canvas.onmousemove = addHover;
    canvas.onmouseout = window.onscroll = removeHover;
  }

  function removeHover() {
    canvas.style.cursor = '';
    highlightEl.style.display = 'none';
    tooltipEl.style.display = 'none';
  }

  function addHover(e) {
    const { i, j } = xyToBar(e.offsetX, e.offsetY);

    if (j === -1 || e.offsetX < 0 || e.offsetX > graphWidth) {
      removeHover();
      return;
    }

    canvas.style.cursor = 'pointer';

    const level = levels[i];
    const x = Math.max(tickToX(level[j]), 0);
    const y = (i - topLevel) * pxPerLevel;
    const sw = Math.min(tickToX(level[j] + level[j + 1]) - x, graphWidth);


    highlightEl.style.display = 'block';
    highlightEl.style.left = (canvas.offsetLeft + x) + 'px';
    highlightEl.style.top = (canvas.offsetTop + y) + 'px';
    highlightEl.style.width = sw + 'px';

    const numBarTicks = level[j + 1];
    const percent = Math.round(10000 * numBarTicks / numTicks) / 100;
    const time = `<div class="time">${percent}%, ${numberWithCommas(numBarTicks)} samples</div>`;
    // let content = names[level[j + 2]];
    let content = `<div class="name">${names[level[j + 2]]}</div>`;
    content += ` ${time}`
    // if (content[0] !== '(') content = content.replace(' ', ` ${time}<br><span class="path">`) + '</span>';
    // else ;

    tooltipEl.innerHTML = content;
    tooltipEl.style.display = 'block';
    tooltipEl.style.left = (Math.min(e.offsetX + 15 + tooltipEl.clientWidth, graphWidth) - tooltipEl.clientWidth) + 'px';
    tooltipEl.style.top = (canvas.offsetTop + e.offsetY + 12) + 'px';
  }

  function roundRect(ctx, x, y, w, h, radius) {
    radius = Math.min(w/2, radius);
    if (radius < 1) {
      return ctx.rect(x,y,w,h);
    }
    var r = x + w;
    var b = y + h;
    ctx.beginPath();
    ctx.moveTo(x + radius, y);
    ctx.lineTo(r - radius, y);
    ctx.quadraticCurveTo(r, y, r, y + radius);
    ctx.lineTo(r, y + h - radius);
    ctx.quadraticCurveTo(r, b, r - radius, b);
    ctx.lineTo(x + radius, b);
    ctx.quadraticCurveTo(x, b, x, b - radius);
    ctx.lineTo(x, y + radius);
    ctx.quadraticCurveTo(x, y, x + radius, y);
  }
}
