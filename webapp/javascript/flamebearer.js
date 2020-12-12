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


export default function render(){
  const introEl = document.getElementById('intro');
  const searchEl = document.getElementById('search');
  const highlightEl = document.getElementById('highlight');
  const tooltipEl = document.getElementById('tooltip');
  const canvas = document.getElementById('flamegraph-canvas');
  const ctx = canvas.getContext('2d');

  let names, levels, numTicks;

  names = [
    "(unknown)",
    "~(anonymous) node:internal/main/run_main_module:1:1",
    "(bytecode) ~Abort.ExtraWide",
    "(unknown)",
    "~prepareMainThreadExecution node:internal/bootstrap/pre_execution:21:36",
    "~patchProcessObject node:internal/bootstrap/pre_execution:78:28",
    "~resolve node:path:973:10",
    "~normalizeString node:path:52:25",
    "~initializeCJSLoader node:internal/bootstrap/pre_execution:413:29",
    "~nativeModuleRequire node:internal/bootstrap/loaders:303:29",
    "~compileForInternalLoader node:internal/bootstrap/loaders:270:27",
    "~(anonymous) node:internal/modules/cjs/loader:1:1",
    "~(anonymous) node:internal/source_map/source_map_cache:1:1",
    "~(anonymous) node:fs:1:1",
    "~(anonymous) node:internal/fs/utils:1:1",
    "~(anonymous) node:internal/modules/package_json_reader:1:1",
    "~(anonymous) node:url:1:1",
    "~executeUserEntryPoint node:internal/modules/run_main:69:31",
    "~resolveMainPath node:internal/modules/run_main:12:25",
    "~Module._findPath node:internal/modules/cjs/loader:479:28",
    "~(anonymous) node:internal/per_context/primordials:23:10",
    "~Module._load node:internal/modules/cjs/loader:742:24",
    "~Module node:internal/modules/cjs/loader:161:16",
    "~Module.load node:internal/modules/cjs/loader:955:33",
    "~Module._extensions..js node:internal/modules/cjs/loader:1111:37",
    "~Module._compile node:internal/modules/cjs/loader:1056:37",
    "~(anonymous) ./app.js:1:1",
    "(C++) __ZN4node9inspector12_GLOBAL__N_120InspectorConsoleCallERKN2v820FunctionCallbackInfoINS2_5ValueEEE",
    "~log node:internal/console/constructor:357:6",
    "~value node:internal/console/constructor:258:20",
    "~Writable.write node:internal/streams/writable:286:36",
    "~writeOrBuffer node:internal/streams/writable:364:23",
    "~SyncWriteStream._write node:internal/fs/sync_write_stream:23:44",
    "~onwrite node:internal/streams/writable:424:17",
    "~nextTick node:internal/process/task_queues:101:18",
    "~setHasTickScheduled node:internal/process/task_queues:49:29",
    "~writeSync node:fs:697:19",
    "~hidden node:internal/errors:308:25",
    "~value node:internal/console/constructor:321:20",
    "~formatWithOptions node:internal/util/inspect:1882:27",
    "~formatWithOptionsInternal node:internal/util/inspect:1890:35",
    "~inspect node:internal/util/inspect:282:17",
    "~once node:events:448:44",
    "~addListener node:events:419:58",
    "~_addListener node:events:357:22",
    "~from node:buffer:290:28",
    "~fromString node:buffer:428:20",
    "~fromStringFast node:buffer:409:24",
    "~write node:buffer:591:12",
    "inspect node:internal/util/inspect:282:17",
    "~formatValue node:internal/util/inspect:736:21",
    "log node:internal/console/constructor:357:6",
    "value node:internal/console/constructor:258:20",
    "formatWithOptions node:internal/util/inspect:1882:27",
    "writeOrBuffer node:internal/streams/writable:364:23",
    "(C++) __ZNSt3__112__hash_tableINS_17__hash_value_typeIN4node13FastStringKeyENS2_17BaseObjectPtrImplINS2_10BaseObjectELb0EEEEENS_22__unordered_map_hasherIS3_S7_NS3_4HashELb1EEENS_21__unordered_map_equalIS3_S7_NS_8equal_toIS3_EELb1EEENS_9allocatorIS7_EEE4findIS3_EENS_15__hash_iteratorIPNS_11__hash_nodeIS7_PvEEEERKT_",
    "(C++) ___psynch_mutexwait",
    "(anonymous) ./app.js:1:1",
    "(C++) __ZN4node7tracing16TraceEventHelper20GetTracingControllerEv",
    "fromStringFast node:buffer:409:24",
    "fromString node:buffer:428:20",
    "formatWithOptionsInternal node:internal/util/inspect:1890:35",
    "(C++) __platform_thread_deallocate",
    "(C++) __platform_task_deallocate",
    "(lib) /usr/lib/system/libsystem_pthread.dylib",
    "formatValue node:internal/util/inspect:736:21",
    "(lib) /usr/lib/system/libsystem_kernel.dylib",
    "(C++) __ZN4node16MaybeStackBufferIN2v85LocalINS1_5ValueEEELm1024EE25AllocateSufficientStorageEm",
    "(lib) /usr/lib/system/libsystem_platform.dylib",
    "_addListener node:events:357:22",
    "from node:buffer:290:28",
    "hidden node:internal/errors:308:25",
    "(anonymous) node:internal/validators:87:3",
    "(C++) _platform_task_copy_next_thread",
    "(C++) __ZN4node6Buffer12_GLOBAL__N_114ByteLengthUtf8ERKN2v820FunctionCallbackInfoINS2_5ValueEEE",
    "(lib) /usr/lib/system/libsystem_malloc.dylib",
    "(C++) __simple_vdprintf",
    "(C++) __ZN4node6Buffer12_GLOBAL__N_111StringWriteILNS_8encodingE1EEEvRKN2v820FunctionCallbackInfoINS4_5ValueEEE",
    "(C++) __ZN4node2fs10GetReqWrapERKN2v820FunctionCallbackInfoINS1_5ValueEEEib",
    "get node:internal/console/constructor:203:14",
    "(C++) __ZN4node2fsL11WriteBufferERKN2v820FunctionCallbackInfoINS1_5ValueEEE",
    "(C++) __ZN4node6Buffer4DataEN2v85LocalINS1_5ValueEEE",
    "(C++) __ZN4node11StringBytes5WriteEPN2v87IsolateEPcmNS1_5LocalINS1_5ValueEEENS_8encodingEPi",
    "(C++) _platform_task_update_threads",
    "(anonymous) node:internal/validators:76:3",
    "(C++) __ZN4node6Buffer4DataEN2v85LocalINS1_6ObjectEEE",
    "(C++) __ZNK4node11Environment14PrintSyncTraceEv",
    "(C++) ___workq_kernreturn",
    "~createPool node:buffer:142:20",
    "~createUnsafeBuffer node:internal/buffer:1048:28",
    "~FastBuffer node:internal/buffer:952:1",
    "~processTicksAndRejections node:internal/process/task_queues:65:35",
    "~afterWriteTick node:internal/streams/writable:481:24",
    "~afterWrite node:internal/streams/writable:486:20",
    "~(anonymous) node:internal/console/constructor:337:10",
    "listenerCount node:events:596:23",
    "afterWrite node:internal/streams/writable:486:20"
  ];
  levels = [
      // basically 3 item tuples:
      // * barIndex, delta encoded
      // * numBarTicks
      // * link to name
      [0,1,0,  0,1558,3 ],
      [0,1,1,  0,1539,1, 0,19,91],
      [0,1,2,  0,1,2,    0,5,4,0,1533,17,0,19,92],
      [0,1,2,  1,2,5,    0,3,8,0,1,18,0,1532,21,0,5,2,0,2,93,0,12,96],
      [2,1,2,  0,1,6,    0,3,9,0,1,19,0,1,22,0,1531,23,5,1,94,0,1,95,7,5,2],
      [3,1,7,  0,3,10,   0,1,20,0,1,2,0,1531,24,5,1,2],
      [3,1,2,  0,3,11,   0,1,2,0,1,3,0,1531,25],
      [3,1,2,  0,1,2,    0,2,9,0,1,2,1,11,2,0,28,26,0,1492,57],
      [4,1,2,  0,2,10,   0,1,2,12,1,2,0,27,27,5,1194,2,0,293,3],
      [5,1,12, 0,1,15,   13,1,2,0,2,2,0,15,28,0,10,51,5,48,2,0,1146,51,0,95,2,0,14,3,0,7,27,0,147,62,0,8,64,0,6,67,0,2,73,0,14,76],
      [5,1,9,  0,1,9,    16,9,29,0,6,38,2,7,52,0,1,53,82,46,2,0,1010,52,0,55,53,0,3,54,0,1,70,0,2,79],
      [5,1,10, 0,1,10,   16,1,2,0,7,30,0,1,42,0,1,2,0,5,39,5,1,45,0,3,54,169,31,2,0,7,45,0,764,54,0,1,60,0,12,69,0,153,70,0,2,79,2,11,2,0,42,61],
      [5,1,13, 0,1,16,   17,5,31,0,2,45,0,1,43,1,5,40,5,1,2,0,3,3,200,1,2,0,1,59,0,5,60,38,10,2,0,700,3,0,11,71,0,1,72,0,4,84,4,9,2,16,4,2,0,133,60,24,33,49],
      [5,1,9,  0,1,2,    17,5,32,0,1,2,0,1,46,0,1,44,1,3,41,0,2,49,6,1,55,0,2,56,203,1,2,0,3,3,48,76,2,0,35,3,0,3,55,0,427,56,0,2,58,0,11,63,0,35,64,0,24,66,0,34,68,0,19,75,0,17,78,0,10,80,0,3,81,0,1,83,0,2,85,0,1,86,8,3,72,49,11,2,0,108,3,0,3,88,34,13,2,0,1,50,0,9,65],
      [5,1,10, 0,1,2,    17,2,33,0,3,36,1,1,47,0,1,2,1,3,2,1,1,50,213,3,2,818,1,2,0,77,2,0,2,3,0,13,64,0,5,74,0,6,77,0,4,82,0,1,87,0,2,2,0,1,89,47,1,2,8,1,2],
      [5,1,14, 18,1,2,   0,1,34,0,1,2,0,2,37,1,1,48,2,3,2,1,1,2,1144,1,2,0,1,90],
      [5,1,2,  19,1,35,  1,2,2,1,1,2,1152,1,2],
      [5,1,2,  19,1,2,   1157,1,2],
      [25,1,2, 1157,1,3]
  ];
  numTicks = 1559;

  let rangeMin = 0;
  let rangeMax = 1;
  let topLevel = 0;
  let query = '';
  let graphWidth, pxPerTick;

  const pxPerLevel = 18;
  const collapseThreshold = 5;
  const hideThreshold = 0.5;
  const labelThreshold = 20;

  highlightEl.style.height = pxPerLevel + 'px';

  if (levels) {
      init();
  }

  function init() {
      document.body.classList.add('loaded');

      // delta-decode bar positions
      for (const level of levels) {
          let prev = 0;
          for (let i = 0; i < level.length; i += 3) {
              level[i] += prev;
              prev = level[i] + level[i + 1];
          }
      }

    //   updateZoom();
      render();
  }

//   window.onhashchange = () => {
//     //   updateZoom();
//       render();
//   };
  canvas.onclick = (e) => {
      const {i, j} = xyToBar(e.offsetX, e.offsetY);
      if (j === -1) return;

    //   window.location.hash = [i, j].join(',');
      updateZoom(i, j);
      render();
      removeHover();
  };
  document.getElementById('reset').onclick = () => {
      searchEl.value = query = '';
      updateZoom(0, 0);
      render();
  };
  window.onresize = render;

  searchEl.oninput = (e) => {
      query = e.target.value;
      render();
  };

//   function updateFromHash() {
//       const [i, j] = window.location.hash.substr(1).split(',').map(Number);

//       if (!isNaN(i) && !isNaN(j)) {
//           topLevel = i;
//           rangeMin = levels[i][j] / numTicks;
//           rangeMax = (levels[i][j] + levels[i][j + 1]) / numTicks;
//       } else {
//           topLevel = 0;
//           rangeMin = 0;
//           rangeMax = 1;
//       }
//   }

  function updateZoom(i, j) {
      if (!isNaN(i) && !isNaN(j)) {
          topLevel = 0;
          rangeMin = levels[i][j] / numTicks;
          rangeMax = (levels[i][j] + levels[i][j + 1]) / numTicks;
      } else {
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
    //   ctx.strokeStyle = '#333';

      for (let i = 0; i < levels.length - topLevel; i++) {
          const level = levels[topLevel + i];

          for (let j = 0; j < level.length; j += 3) {
              const barIndex = level[j];
              const x = tickToX(barIndex);
              const y = i * pxPerLevel;
              let numBarTicks = level[j + 1];

              const inQuery = query && (names[level[j + 2]].indexOf(query) >= 0) || false;

              // merge very small blocks into big "collapsed" ones for performance
              // const collapsed = numBarTicks * pxPerTick <= collapseThreshold;
              const collapsed = false;
              // if (collapsed) {
              //     while (
              //         j < level.length - 3 &&
              //         barIndex + numBarTicks === level[j + 3] &&
              //         level[j + 4] * pxPerTick <= collapseThreshold &&
              //         (inQuery === (query && (names[level[j + 5]].indexOf(query) >= 0) || false))
              //     ) {
              //         j += 3;
              //         numBarTicks += level[j + 1];
              //     }
              // }

              const sw = numBarTicks * pxPerTick - (collapsed ? 0 : 0.5);
              const sh = pxPerLevel - 0.5;

            //   if (x < -1 || x + sw > graphWidth + 1 || sw < hideThreshold) continue;

              ctx.beginPath();
              roundRect(ctx, x, y, sw, sh, 3);

              const ratio = numBarTicks / numTicks;

              if (!collapsed) {
                //   ctx.stroke();
                //   const intensity = Math.min(1, ratio * Math.pow(1.16, i) / (rangeMax - rangeMin));
                //   const h = 50 - 50 * intensity;
                //   const l = 65 + 7 * intensity;

                const rand = murmurhash3_32_gc(names[level[j + 2]]);


                  const r = Math.round(205 + rand%50);
                  const g = Math.round(100 + rand%70);
                  const b = Math.round(0 + rand%55);
                //   ctx.fillStyle = inQuery ? 'lightgreen' : `hsl(${h}, 100%, ${l}%)`;
                  ctx.fillStyle = inQuery ? 'lightgreen' : `rgb(${r},${g},${b})`;
                  console.log(rand, `rgb(${r},${g},${b})`)
              } else {
                  ctx.fillStyle = inQuery ? 'lightgreen' : '#eee';
              }
              ctx.fill();

              if (!collapsed && sw >= labelThreshold) {

                  const percent = Math.round(10000 * ratio) / 100;
                  const name = `${barIndex} ${names[level[j + 2]]} (${percent}%, ${numBarTicks} samples)`;

                  ctx.save();
                  ctx.clip();
                  ctx.fillStyle = 'black';
                  ctx.fillText(name, Math.max(x, 0) + 1, y + sh / 2);
                  ctx.restore();
              }
          }
      }
  }

  // pixel coordinates to bar coordinates in the levels array
  function xyToBar(x, y) {
      const i = Math.floor(y / pxPerLevel) + topLevel;
      const j = binarySearchLevel(x, levels[i]);
      return {i, j};
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
      const {i, j} = xyToBar(e.offsetX, e.offsetY);

      if (j === -1 || e.offsetX < 0 || e.offsetX > graphWidth) {
          removeHover();
          return;
      }

      canvas.style.cursor = 'pointer';

      const level = levels[i];
      const x = tickToX(level[j]);
      const y = (i - topLevel) * pxPerLevel;
      const sw = tickToX(level[j] + level[j + 1]) - x;

      highlightEl.style.display = 'block';
      highlightEl.style.left = x + 'px';
      highlightEl.style.top = (canvas.offsetTop + y) + 'px';
      highlightEl.style.width = sw + 'px';

      const numBarTicks = level[j + 1];
      const percent = Math.round(10000 * numBarTicks / numTicks) / 100;
      const time = `<span class="time">(${percent}%, ${numBarTicks} samples)</span>`;
      let content = names[level[j + 2]];
      if (content[0] !== '(') content = content.replace(' ', ` ${time}<br><span class="path">`) + '</span>';
      else content += ` ${time}`;

      tooltipEl.innerHTML = content;
      tooltipEl.style.display = 'block';
      tooltipEl.style.left = (Math.min(e.offsetX + 15 + tooltipEl.clientWidth, graphWidth) - tooltipEl.clientWidth) + 'px';
      tooltipEl.style.top = (canvas.offsetTop + e.offsetY + 12) + 'px';
  }

  function roundRect(ctx, x, y, w, h, radius){
    var r = x + w;
    var b = y + h;
    ctx.beginPath();
    ctx.moveTo(x+radius, y);
    ctx.lineTo(r-radius, y);
    ctx.quadraticCurveTo(r, y, r, y+radius);
    ctx.lineTo(r, y+h-radius);
    ctx.quadraticCurveTo(r, b, r-radius, b);
    ctx.lineTo(x+radius, b);
    ctx.quadraticCurveTo(x, b, x, b-radius);
    ctx.lineTo(x, y+radius);
    ctx.quadraticCurveTo(x, y, x+radius, y);
  }


}

