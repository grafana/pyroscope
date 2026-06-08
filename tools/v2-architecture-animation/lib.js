// Shared primitives for the Pyroscope v2 architecture animations.
// Each scene (write-path / compaction / read-path) is a standalone HTML file
// that pulls in these helpers and defines its own boxes, flows and timeline.
"use strict";

const NS = "http://www.w3.org/2000/svg";
const INK = "#212529", SUB = "#495057", MUT = "#868e96";
const FLAME = ["#7048e8", "#1c7ed6", "#37b24d", "#f59f00", "#e8590c", "#0ca678", "#ae3ec9", "#1098ad"];

function el(tag, attrs, parent) {
  const e = document.createElementNS(NS, tag);
  if (attrs) for (const k in attrs) e.setAttribute(k, attrs[k]);
  if (parent) parent.appendChild(e);
  return e;
}

const clamp = (x, a, b) => Math.max(a, Math.min(b, x));
const lerp = (a, b, t) => a + (b - a) * t;
const easeInOut = t => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);

// progress within sub-interval [a,b], optionally eased
function seg(t, a, b, ease = true) {
  const x = clamp((t - a) / (b - a), 0, 1);
  return ease ? easeInOut(x) : x;
}

// position at fraction f along a polyline of [x,y] points
function pointAt(pts, f) {
  f = clamp(f, 0, 1);
  let total = 0; const segs = [];
  for (let i = 0; i < pts.length - 1; i++) {
    const d = Math.hypot(pts[i + 1][0] - pts[i][0], pts[i + 1][1] - pts[i][1]);
    segs.push(d); total += d;
  }
  let dist = f * total;
  for (let i = 0; i < segs.length; i++) {
    if (dist <= segs[i] || i === segs.length - 1) {
      const u = segs[i] === 0 ? 0 : dist / segs[i];
      return [lerp(pts[i][0], pts[i + 1][0], u), lerp(pts[i][1], pts[i + 1][1], u)];
    }
    dist -= segs[i];
  }
  return pts[pts.length - 1];
}

// edge/center anchors for a box {x,y,w,h}
function anchors(b) {
  return {
    cx: b.x + b.w / 2, cy: b.y + b.h / 2,
    l: [b.x, b.y + b.h / 2], r: [b.x + b.w, b.y + b.h / 2],
    t: [b.x + b.w / 2, b.y], bot: [b.x + b.w / 2, b.y + b.h],
    box: b,
  };
}

function drawBox(parent, b, lines, opts = {}) {
  const g = el("g", {}, parent);
  el("rect", { x: b.x, y: b.y, width: b.w, height: b.h, rx: 13, ry: 13,
    fill: b.fill, stroke: INK, "stroke-width": 2.5 }, g);
  if (opts.accent) el("rect", { x: b.x, y: b.y, width: b.w, height: 7, rx: 3, ry: 3, fill: opts.accent }, g);
  let ty = b.y + (opts.titleOffset || 34);
  lines.forEach(ln => {
    el("text", { x: b.x + b.w / 2, y: ty, "text-anchor": "middle",
      "font-size": ln.size || 19, "font-weight": ln.bold ? 700 : 500, fill: ln.fill || INK }, g)
      .textContent = ln.t;
    ty += (ln.gap || (ln.size ? ln.size + 7 : 26));
  });
  return g;
}

function pathD(pts) {
  let d = "M " + pts[0][0] + " " + pts[0][1];
  for (let i = 1; i < pts.length; i++) d += " L " + pts[i][0] + " " + pts[i][1];
  return d;
}

// dashed connector with arrowhead at the end
function conn(parent, pts, opts = {}) {
  el("path", { d: pathD(pts), fill: "none", stroke: opts.stroke || "#adb5bd",
    "stroke-width": opts.w || 2, "stroke-dasharray": opts.dash || "7 6",
    "stroke-linecap": "round", "stroke-linejoin": "round" }, parent);
  const p2 = pts[pts.length - 1], p1 = pts[pts.length - 2];
  const ang = Math.atan2(p2[1] - p1[1], p2[0] - p1[0]);
  const s = 9, a1 = ang + Math.PI - 0.5, a2 = ang + Math.PI + 0.5;
  el("path", { d: `M ${p2[0] + s * Math.cos(a1)} ${p2[1] + s * Math.sin(a1)} L ${p2[0]} ${p2[1]} L ${p2[0] + s * Math.cos(a2)} ${p2[1] + s * Math.sin(a2)}`,
    fill: "none", stroke: opts.arrow || "#868e96", "stroke-width": 2.2,
    "stroke-linecap": "round", "stroke-linejoin": "round" }, parent);
}

// deterministic flame-graph row shapes
function rows(seed, levels) {
  const out = [];
  let s = seed * 9301 + 49297;
  const rnd = () => { s = (s * 9301 + 49297) % 233280; return s / 233280; };
  for (let l = 0; l < levels; l++) {
    const n = 2 + Math.floor(rnd() * 3);
    const parts = []; let rem = 1;
    for (let i = 0; i < n; i++) {
      const v = i === n - 1 ? rem : Math.max(0.12, rnd() * rem * 0.7);
      parts.push(v); rem -= v;
    }
    out.push(parts);
  }
  return out;
}

// mini flame graph anchored at baseline (x,y), growing upward; reveals bottom-up
function makeFlame(parent, x, y, w, rws, seed) {
  const g = el("g", {}, parent);
  const rh = 9, gap = 2, all = [];
  let pal = seed || 0;
  for (let r = 0; r < rws.length; r++) {
    let cx = x;
    for (let s = 0; s < rws[r].length; s++) {
      const frac = rws[r][s];
      all.push(el("rect", { x: cx, y: y - r * (rh + gap), width: Math.max(2, w * frac - 1),
        height: rh, rx: 1.5, fill: FLAME[(pal++) % FLAME.length] }, g));
      cx += w * frac;
    }
  }
  return {
    g,
    setReveal(rv) { const n = Math.round(rv * all.length); all.forEach((rc, i) => rc.setAttribute("opacity", i < n ? 1 : 0)); },
    setOpacity(o) { g.setAttribute("opacity", o); },
  };
}

// flame-graph (icicle) layout: a single full-width root on top, narrowing
// downward into segmented children. Returns levels of {x, w}.
function flameLayout(W, depth, seed) {
  let s = (seed * 2654435761) >>> 0;
  const rnd = () => { s = (s * 1664525 + 1013904223) >>> 0; return (s >>> 8) / 16777216; };
  const levels = [[{ x: 0, w: W }]];
  for (let l = 1; l < depth; l++) {
    const cur = [];
    for (const f of levels[l - 1]) {
      if (f.w < W * 0.12) continue;            // too thin to have children
      let avail = f.w * (0.5 + rnd() * 0.45);  // children cover 50-95% of parent
      let cx = f.x + rnd() * (f.w - avail) * 0.6;
      const n = 1 + Math.floor(rnd() * 3);
      for (let c = 0; c < n && avail > W * 0.05; c++) {
        const cw = Math.min(avail, c === n - 1 ? avail : Math.max(W * 0.05, avail * (0.35 + rnd() * 0.5)));
        cur.push({ x: cx, w: cw });
        cx += cw; avail -= cw;
      }
    }
    if (!cur.length) break;
    levels.push(cur);
  }
  return levels;
}

// a small flame-graph icon (root on top) that can travel; .at(x,y) centers it
function makeIcicle(parent, w, depth, seed, rh = 4) {
  const g = el("g", { opacity: 0 }, parent);
  const gap = 1.2, levels = flameLayout(w, depth, seed);
  let pal = seed;
  levels.forEach((lvl, l) => lvl.forEach(f =>
    el("rect", { x: f.x, y: l * (rh + gap), width: Math.max(2, f.w - 1), height: rh,
      rx: 1, fill: FLAME[(pal++) % FLAME.length] }, g)));
  const h = levels.length * (rh + gap);
  return {
    at(x, y, op = 1) { g.setAttribute("transform", `translate(${x - w / 2},${y - h / 2})`); g.setAttribute("opacity", op); },
    hide() { g.setAttribute("opacity", 0); },
  };
}

// a flame graph (icicle) anchored at top-left (x,y), revealed top-down by level
function staticIcicle(parent, x, y, w, depth, seed, rh = 6) {
  const g = el("g", {}, parent);
  const gap = 1.5, levels = flameLayout(w, depth, seed);
  let pal = seed;
  const rects = [];
  levels.forEach((lvl, l) => lvl.forEach(f =>
    rects.push({ lvl: l, el: el("rect", { x: x + f.x, y: y + l * (rh + gap), width: Math.max(2, f.w - 1),
      height: rh, rx: 1, fill: FLAME[(pal++) % FLAME.length], opacity: 0 }, g) })));
  const depthShown = levels.length;
  return {
    g,
    setReveal(rv) { const n = rv * depthShown; rects.forEach(r => r.el.setAttribute("opacity", r.lvl < n ? 1 : 0)); },
    setOpacity(o) { g.setAttribute("opacity", o); },
  };
}

// moving data marker (rounded square)
function marker(parent, color, size = 15) {
  const r = el("rect", { x: -100, y: -100, width: size, height: size, rx: 2.5,
    fill: color, stroke: INK, "stroke-width": 1.6, opacity: 0 }, parent);
  return {
    at(x, y, op = 1) { r.setAttribute("x", x - size / 2); r.setAttribute("y", y - size / 2); r.setAttribute("opacity", op); },
    hide() { r.setAttribute("opacity", 0); },
  };
}

// drive a scene: ?t=<v> renders one static frame (for capture); otherwise it loops
function mount(seekFn, durMs) {
  const p = new URLSearchParams(location.search);
  if (p.has("t")) {
    seekFn(parseFloat(p.get("t")));
    document.title = "frame-ready";
  } else {
    const tick = ts => { seekFn((ts % durMs) / durMs); requestAnimationFrame(tick); };
    requestAnimationFrame(tick);
  }
  window.seek = seekFn;
}
