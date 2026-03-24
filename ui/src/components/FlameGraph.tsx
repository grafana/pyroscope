import { useState } from 'react';
import type { Frame } from '@hooks/usePyroscopeQuery';

const FRAME_H = 22;

export function FlameGraph({ levels }: { levels: Frame[][] }) {
  const [hovered, setHovered] = useState<{ name: string; pct: number } | null>(
    null,
  );

  return (
    <div>
      {levels.map((level, li) => (
        <div
          key={li}
          style={{ position: 'relative', height: FRAME_H, marginBottom: 1 }}
        >
          {level.map((frame) => {
            const isHov = hovered?.name === frame.name;
            return (
              <div
                key={`${li}-${frame.name}`}
                onMouseEnter={() =>
                  setHovered({ name: frame.name, pct: frame.width })
                }
                onMouseLeave={() => setHovered(null)}
                style={{
                  position: 'absolute',
                  left: `${frame.start}%`,
                  width: `calc(${frame.width}% - 1px)`,
                  height: '100%',
                  background: frameColor(frame.name),
                  filter: isHov ? 'brightness(1.25)' : undefined,
                  cursor: 'pointer',
                  overflow: 'hidden',
                  display: 'flex',
                  alignItems: 'center',
                  paddingLeft: 4,
                  borderRadius: 1,
                }}
              >
                {frame.width > 2.5 && (
                  <span
                    style={{
                      fontSize: 'var(--text-xs)',
                      color: 'rgba(255,255,255,0.88)',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      pointerEvents: 'none',
                      userSelect: 'none' as const,
                      lineHeight: 1,
                    }}
                  >
                    {frame.name}
                  </span>
                )}
              </div>
            );
          })}
        </div>
      ))}

      <div
        style={{
          marginTop: 'var(--space-2)',
          paddingTop: 'var(--space-2)',
          borderTop: '1px solid var(--border-weak)',
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
              {hovered.pct}%
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
    </div>
  );
}

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
