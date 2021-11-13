import { Option } from 'prelude-ts';
import React from 'react';
import { DeepReadonly } from 'ts-essentials';
import styles from './Highlight.module.css';

export interface HighlightProps {
  // probably the same as the bar height
  barHeight: number;
  zoom: Option<DeepReadonly<{ i: number; j: number }>>;
  xyToHighlightData: (
    x: number,
    y: number
  ) => Option<{
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

  const onMouseMove = (e: MouseEvent) => {
    const opt = xyToHighlightData(e.offsetX, e.offsetY);

    if (opt.isSome()) {
      const data = opt.get();

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
  };

  const onMouseOut = () => {
    setStyle({
      ...style,
      visibility: 'hidden',
    });
  };

  React.useEffect(
    () => {
      // use closure to "cache" the current canvas reference
      // so that when cleaning up, it points to a valid canvas
      // (otherwise it would be null)
      const canvasEl = canvasRef.current;
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
    [canvasRef.current, onMouseMove, onMouseOut]
  );

  return (
    <div
      className={styles.highlight}
      style={style}
      data-testid="flamegraph-highlight"
    />
  );
}
