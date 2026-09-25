#!/usr/bin/env node
// Generates turtleFieldPlacements.ts. Run by hand; the output is committed and
// this is not wired into the build:
//
//   node scripts/generateTurtleField.mjs ".claude/misc/output (1).svg"
//
// Provenance: the input tiling is deliberately NOT in this repo. .gitignore
// excludes .claude/, and the 1.5MB source is never needed at runtime (only the
// emitted .ts is), so putting it in public/ would have Next serve it for no
// reason. Same arrangement as tunnelTilePatch.ts, whose hatviz-derived source
// was likewise external.

import fs from "node:fs";

const KITES_SVG =
  "KaleidoscopeFrontend/kaleidoscope/public/turtle-monotile-kites.svg";
const OUT_TS =
  "KaleidoscopeFrontend/kaleidoscope/components/KscopeSharedUI/KaleidoscopeTunnel/turtleFieldPlacements.ts";

const SQRT3 = Math.sqrt(3);

const dist = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]);

function die(msg) {
  console.error(`\n  ABORT: ${msg}\n`);
  process.exit(1);
}

// --- canonical turtle, derived from the kite artwork ------------------------

// Parses the subset of path syntax these files actually use: one absolute
// moveto, then relative linetos, then a close. The final lineto returns to the
// start point, so that duplicate is dropped.
function parseKitePaths(svg) {
  return [...svg.matchAll(/<path[^>]*\sd="([^"]+)"/g)].map((m) => {
    const tok = m[1].trim().split(/[\s,]+/);
    const pts = [];
    let x = 0;
    let y = 0;
    for (let i = 0; i < tok.length; ) {
      const t = tok[i];
      if (t === "M") {
        x = +tok[i + 1];
        y = +tok[i + 2];
        pts.push([x, y]);
        i += 3;
      } else if (t === "l" || t === "z" || t === "Z") {
        i += 1;
      } else {
        x += +tok[i];
        y += +tok[i + 1];
        pts.push([x, y]);
        i += 2;
      }
    }
    // The closing lineto lands ~1e-8 off the start point, since these paths
    // are written as 8-decimal relative steps that accumulate error.
    if (pts.length > 1 && dist(pts[0], pts[pts.length - 1]) < 1e-6) pts.pop();
    return pts;
  });
}

// Every edge interior to the turtle is shared by two kites and so appears
// twice; the outline is exactly the edges appearing once. Deriving the outline
// this way -- rather than hardcoding it, or reading the separate
// turtle-monotile.svg -- locks the placement math and the rendered artwork to
// one file, so they cannot silently drift apart.
//
// The walk is undirected because the kites are not consistently wound; a shared
// edge can appear in the same direction from both neighbours.
function outlineFromKites(kites) {
  const key = (p) => `${p[0].toFixed(6)},${p[1].toFixed(6)}`;
  const seen = new Map();
  for (const k of kites) {
    for (let i = 0; i < k.length; i++) {
      const a = k[i];
      const b = k[(i + 1) % k.length];
      const id = [key(a), key(b)].sort().join("|");
      if (seen.has(id)) seen.delete(id);
      else seen.set(id, [a, b]);
    }
  }

  const adj = new Map();
  for (const [, [a, b]] of seen) {
    if (!adj.has(key(a))) adj.set(key(a), []);
    if (!adj.has(key(b))) adj.set(key(b), []);
    adj.get(key(a)).push(b);
    adj.get(key(b)).push(a);
  }
  for (const [k, v] of adj) {
    if (v.length !== 2) die(`kite outline is not a simple cycle at ${k}`);
  }

  const start = seen.values().next().value[0];
  const cycle = [start];
  let prev = start;
  let cur = adj.get(key(start))[0];
  while (key(cur) !== key(start)) {
    cycle.push(cur);
    const [n0, n1] = adj.get(key(cur));
    const next = key(n0) === key(prev) ? n1 : n0;
    prev = cur;
    cur = next;
  }
  if (cycle.length !== adj.size) die("kite outline visited only part of the boundary");
  return cycle;
}

// --- the turtle guard -------------------------------------------------------

// A hat file already slipped through once, so this stays permanent rather than
// trusting a filename. Hat and turtle are both Tile(a,b) with the same sqrt(3)
// edge ratio, so the ratio alone proves nothing. What separates them is which
// edge is which: the turtle has 6 short / 8 long (the hat is the reverse, and
// that tally survives rotation and reflection), and the turtle's one collinear
// vertex triple sits on a doubled *long* edge where the hat's is doubled short.
function classify(poly) {
  const n = poly.length;
  const lens = poly.map((p, i) => dist(p, poly[(i + 1) % n]));
  const s = Math.min(...lens);
  const L = Math.max(...lens);
  const tol = s * 1e-3;
  const nShort = lens.filter((v) => Math.abs(v - s) < tol).length;
  const nLong = lens.filter((v) => Math.abs(v - L) < tol).length;

  let doubled = null;
  for (let i = 0; i < n; i++) {
    const a = poly[i];
    const b = poly[(i + 1) % n];
    const c = poly[(i + 2) % n];
    const cross =
      (b[0] - a[0]) * (c[1] - b[1]) - (b[1] - a[1]) * (c[0] - b[0]);
    if (Math.abs(cross) < s * s * 1e-6) doubled = dist(a, b);
  }
  return { short: s, long: L, ratio: L / s, nShort, nLong, doubled };
}

function assertTurtle(poly, what) {
  const c = classify(poly);
  if (poly.length !== 14) die(`${what}: expected 14 vertices, got ${poly.length}`);
  if (Math.abs(c.ratio - SQRT3) > 1e-3)
    die(`${what}: edge ratio ${c.ratio.toFixed(5)} is not sqrt(3)`);
  if (c.nShort !== 6 || c.nLong !== 8)
    die(
      `${what}: ${c.nShort} short / ${c.nLong} long -- the turtle is 6/8, ` +
        `the hat is 8/6. This looks like a HAT tiling.`,
    );
  if (c.doubled === null) die(`${what}: no collinear vertex triple found`);
  if (Math.abs(c.doubled - c.long) > c.short * 1e-3)
    die(
      `${what}: doubled edge is ${c.doubled.toFixed(3)} (short=${c.short.toFixed(3)}, ` +
        `long=${c.long.toFixed(3)}). The turtle's doubled edge is 2x LONG; ` +
        `the hat's is 2x short. This looks like a HAT tiling.`,
    );
  return c;
}

// --- fitting each tiling polygon back to the canonical turtle ---------------

// Recovers the (mirror, rotate, translate) that carries the canonical turtle
// onto one tiling polygon, in the same order tunnelSpiral.ts uses:
// p' = R(rot) . S(mirror) . p. Brute force over every vertex correspondence --
// 14 cyclic offsets x both mirror states x both traversal directions -- keeping
// the least-squares best. Exhaustive is fine at this size and removes any need
// to reason about which vertex of the source polygon is "first".
function fitTurtle(canon, target) {
  const n = canon.length;
  let best = null;

  for (const mirror of [1, -1]) {
    const q = canon.map(([x, y]) => [mirror * x, y]);
    const qc = [
      q.reduce((a, p) => a + p[0], 0) / n,
      q.reduce((a, p) => a + p[1], 0) / n,
    ];
    const tc = [
      target.reduce((a, p) => a + p[0], 0) / n,
      target.reduce((a, p) => a + p[1], 0) / n,
    ];

    for (const dir of [1, -1]) {
      for (let k = 0; k < n; k++) {
        let sxy = 0;
        let sxx = 0;
        for (let i = 0; i < n; i++) {
          const t = target[(((k + dir * i) % n) + n) % n];
          const qx = q[i][0] - qc[0];
          const qy = q[i][1] - qc[1];
          const px = t[0] - tc[0];
          const py = t[1] - tc[1];
          sxy += qx * py - qy * px;
          sxx += qx * px + qy * py;
        }
        const theta = Math.atan2(sxy, sxx);
        const c = Math.cos(theta);
        const s = Math.sin(theta);

        let resid = 0;
        for (let i = 0; i < n; i++) {
          const t = target[(((k + dir * i) % n) + n) % n];
          const qx = q[i][0] - qc[0];
          const qy = q[i][1] - qc[1];
          const rx = c * qx - s * qy;
          const ry = s * qx + c * qy;
          resid += (rx - (t[0] - tc[0])) ** 2 + (ry - (t[1] - tc[1])) ** 2;
        }
        if (best === null || resid < best.resid) {
          // The turtle's V0 is the origin of the canonical outline, and both
          // mirror and rotation fix the origin, so the translation is exactly
          // where V0 lands.
          best = {
            resid,
            theta,
            mirror,
            tx: tc[0] - (c * qc[0] - s * qc[1]),
            ty: tc[1] - (s * qc[0] + c * qc[1]),
          };
        }
      }
    }
  }
  return best;
}

// --- main -------------------------------------------------------------------

const inputPath = process.argv[2];
if (!inputPath) die("usage: node scripts/generateTurtleField.mjs <tiling.svg>");
if (!fs.existsSync(inputPath)) die(`no such file: ${inputPath}`);

const kites = parseKitePaths(fs.readFileSync(KITES_SVG, "utf8"));
if (kites.length !== 10)
  die(`${KITES_SVG}: expected 10 kites, found ${kites.length} -- a turtle is ` +
      `10 kites, a hat is 8.`);

const canon = outlineFromKites(kites);
const canonInfo = assertTurtle(canon, "canonical outline from kites");
console.log(`canonical turtle: ${canon.length} vertices, ` +
            `short=${canonInfo.short.toFixed(4)} long=${canonInfo.long.toFixed(4)}`);

const svg = fs.readFileSync(inputPath, "utf8");

const vb = svg.match(/viewBox="([^"]+)"/);
if (!vb) die("input has no viewBox");
const [vbX, vbY, vbW, vbH] = vb[1].trim().split(/[\s,]+/).map(Number);

// Polygon coordinates are in the <g>'s translated frame, so the visible
// rectangle has to be expressed in that same frame.
const tr = svg.match(/<g[^>]*transform="translate\(\s*([-\d.]+)\s*,\s*([-\d.]+)\s*\)"/);
if (!tr) die("input has no <g transform=\"translate(...)\">");
const [tX, tY] = [Number(tr[1]), Number(tr[2])];
const frame = { x0: vbX - tX, y0: vbY - tY, x1: vbX + vbW - tX, y1: vbY + vbH - tY };

const polys = [...svg.matchAll(/points="([^"]+)"/g)].map((m) =>
  m[1].trim().split(/\s+/).map((p) => p.split(",").map(Number)),
);
console.log(`input: ${polys.length} polygons`);

// One uniform tile means one uniform scale; take it from the first polygon and
// hold every other polygon to it, so a mixed-scale file fails loudly.
const scale = assertTurtle(polys[0], "polygon 0").short / canonInfo.short;
console.log(`scale: ${scale.toFixed(5)} units per short edge`);

let maxResid = 0;
let maxSnap = 0;
const kept = [];

for (let i = 0; i < polys.length; i++) {
  const info = assertTurtle(polys[i], `polygon ${i}`);
  if (Math.abs(info.short / canonInfo.short - scale) > scale * 1e-3)
    die(`polygon ${i}: scale ${(info.short / canonInfo.short).toFixed(5)} ` +
        `differs from ${scale.toFixed(5)}`);

  // Whole tiles only. Clipping a polygon at the frame would render a cut
  // turtle, which reads as a bug rather than a crop; overlapping tiles are
  // kept intact so the field overfills and `slice` can never expose an edge.
  const xs = polys[i].map((p) => p[0]);
  const ys = polys[i].map((p) => p[1]);
  if (Math.max(...xs) < frame.x0 || Math.min(...xs) > frame.x1) continue;
  if (Math.max(...ys) < frame.y0 || Math.min(...ys) > frame.y1) continue;

  const unit = polys[i].map(([x, y]) => [x / scale, y / scale]);
  const fit = fitTurtle(canon, unit);
  maxResid = Math.max(maxResid, Math.sqrt(fit.resid / canon.length));

  // Every edge direction in this tiling is a multiple of 30 degrees, so
  // snapping removes float noise without moving any tile.
  const deg = (fit.theta * 180) / Math.PI;
  const snapped = Math.round(deg / 30) * 30;
  maxSnap = Math.max(maxSnap, Math.abs(deg - snapped));

  kept.push({
    x: +fit.tx.toFixed(4),
    y: +fit.ty.toFixed(4),
    rot: ((snapped % 360) + 360) % 360,
    mirrored: fit.mirror === -1,
  });
}

console.log(`kept: ${kept.length} tiles overlapping the viewBox`);
console.log(`max fit residual: ${maxResid.toExponential(2)} (canonical units)`);
console.log(`max rotation snap: ${maxSnap.toExponential(2)} degrees`);

if (maxResid > 1e-6) die(`fit residual too large -- a polygon is not the canonical turtle`);
if (maxSnap > 1e-6) die(`rotations are not multiples of 30 degrees`);

const view = {
  x: +(frame.x0 / scale).toFixed(4),
  y: +(frame.y0 / scale).toFixed(4),
  w: +((frame.x1 - frame.x0) / scale).toFixed(4),
  h: +((frame.y1 - frame.y0) / scale).toFixed(4),
};

const header = `// GENERATED by scripts/generateTurtleField.mjs -- do not hand-edit.
//
// Source: an aperiodic turtle-monotile tiling of ${polys.length} tiles, kept
// outside this repo (see the generator's header for why). Only the ${kept.length}
// tiles overlapping that file's own viewBox are emitted; the rest lie outside
// the frame and could never be drawn, so shipping them would cost bundle size
// for pixel-identical output.
//
// Units: 1 = one short tile edge, matching turtle-monotile-kites-fresnel.svg's
// own coordinate system, so a placement can drive an <image> directly.
//
// Placement composes as p' = R(rot) . S(mirrored) . p, the same
// mirror-then-rotate order as tunnelTilePatch.ts / tunnelSpiral.ts, which is
// also exactly what SVG's translate() rotate() scale() does. rot is degrees
// clockwise on screen; the pivot is the turtle's V0, at the artwork's origin.

export type TurtleTile = {
  readonly x: number;
  readonly y: number;
  readonly rot: number;
  readonly mirrored: boolean;
};

// The rectangle the placements were selected to cover, in the same units.
export const TURTLE_FIELD_VIEWBOX = ${JSON.stringify(view)} as const;

export const TURTLE_FIELD_PLACEMENTS: readonly TurtleTile[] = [
`;

const body = kept
  .map((t) => `  { x: ${t.x}, y: ${t.y}, rot: ${t.rot}, mirrored: ${t.mirrored} },`)
  .join("\n");

fs.writeFileSync(OUT_TS, `${header}${body}\n];\n`, "utf8");
console.log(`wrote ${OUT_TS}`);
