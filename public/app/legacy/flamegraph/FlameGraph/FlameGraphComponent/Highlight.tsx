import { Maybe } from 'true-myth';
import React, { useCallback } from 'react';
import { DeepReadonly } from 'ts-essentials';
import styles from './Highlight.module.css';

export interface HighlightProps {
  // probably the same as the bar height
  barHeight: number;
  zoom: Maybe<DeepReadonly<{ i: number; j: number }>>;
  xyToHighlightData: (
    x: number,
    y: number
  ) => Maybe<{
    left: number;
    top: number;
    width: number;
  }>;

  canvasRef: React.RefObject<HTMLCanvasElement>;
}
export default function Highlight(props: HighlightProps) {
  const { canvasRef, barHeight, xyToHighlightData, zoom } = props;
  const [style, setStyle] = React.useState<React.CSSProperties>({
    height: '0px',
    visibility: 'hidden',
  });

  React.useEffect(() => {
    // stops highlighting every time a node is zoomed or unzoomed
    // then, when a mouse move event is detected,
    // listeners are triggered and highlighting becomes visible again
    setStyle({
      height: '0px',
      visibility: 'hidden',
    });
  }, [zoom]);

  const onMouseOut = useCallback(() => {
    setStyle({
      ...style,
      visibility: 'hidden',
    });
  }, [setStyle, style]);

  const onMouseMove = useCallback(
    (e: MouseEvent) => {
      const opt = xyToHighlightData(e.offsetX, e.offsetY);

      if (opt.isJust) {
        const data = opt.value;

        setStyle({
          visibility: 'visible',
          height: `${barHeight}px`,
          ...data,
        });
      } else {
        // it doesn't map to a valid xy
        // so it means we are hovering out
        onMouseOut();
      }
    },
    [setStyle, onMouseOut, barHeight, xyToHighlightData]
  );

  const canvasEl = canvasRef.current;

  React.useEffect(
    () => {
      // use closure to "cache" the current canvas reference
      // so that when cleaning up, it points to a valid canvas
      // (otherwise it would be null)
      if (!canvasEl) {
        return () => {};
      }

      // watch for mouse events on the bar
      canvasEl.addEventListener('mousemove', onMouseMove);
      canvasEl.addEventListener('mouseout', onMouseOut);

      return () => {
        canvasEl.removeEventListener('mousemove', onMouseMove);
        canvasEl.removeEventListener('mouseout', onMouseOut);
      };
    },

    // refresh callback functions when they change
    [canvasEl, onMouseMove, onMouseOut]
  );

  return (
    <div
      className={styles.highlight}
      style={style}
      data-testid="flamegraph-highlight"
    />
  );
}
