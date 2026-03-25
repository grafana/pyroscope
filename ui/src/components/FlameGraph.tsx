import { useState, useRef, useEffect } from 'react';
import type { Frame } from '@hooks/usePyroscopeQuery';

const FRAME_H = 22;
const ROW_GAP = 1;
const ROW_H = FRAME_H + ROW_GAP;
const LABEL_MIN_PX = 20;
const LABEL_PAD_PX = 4;
const BRIGHTNESS_BOOST = 14;

function djb2(s: string) {
  let h = 5381;
  for (let i = 0; i < s.length; i++)
    h = ((h * 33) ^ s.charCodeAt(i)) & 0x7fffffff;
  return h;
}

function frameColor(name: string): string {
  const h = djb2(name);
  if (/gc|GC|grey|malloc/.test(name))
    return `hsl(${2 + (h % 8)},  65%, ${32 + (h % 10)}%)`;
  if (name.startsWith('runtime.'))
    return `hsl(${20 + (h % 12)}, 68%, ${34 + (h % 10)}%)`;
  return `hsl(${28 + (h % 22)}, 72%, ${36 + (h % 10)}%)`;
}

function frameColorHovered(name: string): string {
  const base = frameColor(name);
  const m = base.match(/hsl\((\d+),\s*(\d+)%,\s*(\d+)%\)/);
  if (!m) return base;
  const h = parseInt(m[1]);
  const s = parseInt(m[2]);
  const l = Math.min(90, parseInt(m[3]) + BRIGHTNESS_BOOST);
  return `hsl(${h}, ${s}%, ${l}%)`;
}

function draw(
  canvas: HTMLCanvasElement,
  cssWidth: number,
  levels: Frame[][],
  hovered: { name: string; pct: number } | null,
) {
  const dpr = window.devicePixelRatio || 1;
  const cssHeight =
    levels.length === 0 ? 0 : levels.length * ROW_H - ROW_GAP;

  canvas.width = cssWidth * dpr;
  canvas.height = cssHeight * dpr;
  canvas.style.width = `${cssWidth}px`;
  canvas.style.height = `${cssHeight}px`;

  if (levels.length === 0) return;

  const ctx = canvas.getContext('2d')!;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

  const rootStyle = getComputedStyle(document.documentElement);
  const fontFamily = rootStyle.getPropertyValue('--font-mono').trim();
  const rootFontSizePx = parseFloat(getComputedStyle(document.documentElement).fontSize);
  const textXsRem = parseFloat(rootStyle.getPropertyValue('--text-xs').trim());
  const fontSizePx = Math.round(textXsRem * rootFontSizePx);
  ctx.font = `${fontSizePx}px ${fontFamily}`;
  ctx.textBaseline = 'middle';

  for (let li = 0; li < levels.length; li++) {
    const level = levels[li];
    for (const frame of level) {
      const x = (frame.start / 100) * cssWidth;
      const w = Math.max(0, (frame.width / 100) * cssWidth - 1);
      if (w < 0.5) continue;
      const y = li * ROW_H;

      const isHov = hovered?.name === frame.name;
      ctx.fillStyle = isHov ? frameColorHovered(frame.name) : frameColor(frame.name);
      ctx.beginPath();
      ctx.roundRect(x, y, w, FRAME_H, 1);
      ctx.fill();

      if (w > LABEL_MIN_PX) {
        ctx.save();
        ctx.beginPath();
        ctx.rect(x, y, w, FRAME_H);
        ctx.clip();
        ctx.fillStyle = 'rgba(255,255,255,0.88)';
        ctx.fillText(frame.name, x + LABEL_PAD_PX, y + FRAME_H / 2);
        ctx.restore();
      }
    }
  }
}

function hitTest(
  x: number,
  y: number,
  levels: Frame[][],
  cssWidth: number,
): { name: string; pct: number } | null {
  const li = Math.floor(y / ROW_H);
  if (li < 0 || li >= levels.length) return null;
  for (const frame of levels[li]) {
    const fx = (frame.start / 100) * cssWidth;
    const fw = (frame.width / 100) * cssWidth;
    if (x >= fx && x < fx + fw) return { name: frame.name, pct: frame.width };
  }
  return null;
}

export function FlameGraph({ levels }: { levels: Frame[][] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const levelsRef = useRef(levels);
  const [hovered, setHovered] = useState<{ name: string; pct: number } | null>(null);
  const hoveredRef = useRef(hovered);

  useEffect(() => { levelsRef.current = levels; });
  useEffect(() => { hoveredRef.current = hovered; });

  useEffect(() => {
    const ro = new ResizeObserver((entries) => {
      const w = entries[0].contentRect.width;
      draw(canvasRef.current!, w, levelsRef.current, hoveredRef.current);
    });
    ro.observe(containerRef.current!);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    draw(canvasRef.current!, containerRef.current!.clientWidth, levels, hoveredRef.current);
  }, [levels]);

  function handleMouseMove(e: React.MouseEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current!;
    const cssWidth = containerRef.current!.clientWidth;
    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    const hit = hitTest(x, y, levelsRef.current, cssWidth);
    if (hit?.name !== hoveredRef.current?.name) {
      canvas.style.cursor = hit ? 'pointer' : 'default';
      draw(canvas, cssWidth, levelsRef.current, hit);
      setHovered(hit);
    }
  }

  function handleMouseLeave() {
    if (hoveredRef.current !== null) {
      const canvas = canvasRef.current!;
      draw(canvas, containerRef.current!.clientWidth, levelsRef.current, null);
      canvas.style.cursor = 'default';
      setHovered(null);
    }
  }

  return (
    <div ref={containerRef} style={{ width: '100%' }}>
      <div
        style={{
          marginBottom: 'var(--space-2)',
          paddingBottom: 'var(--space-2)',
          borderBottom: '1px solid var(--border-weak)',
          minHeight: 20,
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--space-3)',
        }}
      >
        {hovered ? (
          <>
            <span
              style={{
                fontSize: 'var(--text-xs)',
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-primary)',
              }}
            >
              {hovered.name}
            </span>
            <span
              style={{
                fontSize: 'var(--text-xs)',
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-secondary)',
              }}
            >
              {hovered.pct.toFixed(2)}%
            </span>
          </>
        ) : (
          <span
            style={{
              fontSize: 'var(--text-xs)',
              color: 'var(--text-disabled)',
            }}
          >
            Hover a frame to inspect
          </span>
        )}
      </div>
      <canvas
        ref={canvasRef}
        style={{ display: 'block' }}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
      />
    </div>
  );
}
