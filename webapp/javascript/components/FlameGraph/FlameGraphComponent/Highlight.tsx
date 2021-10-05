import React from 'react';

export interface HighlightProps {
  isWithinBounds: (x: number, y: number) => boolean;

  // probably the same as the bar height
  barHeight: number;

  xyToHighlightData: (
    x: number,
    y: number
  ) => {
    left: number;
    top: number;
    width: number;
  };

  canvasRef: React.RefObject<HTMLCanvasElement>;
}
export default function Highlight(props: HighlightProps) {
  const { canvasRef, isWithinBounds, barHeight, xyToHighlightData } = props;
  const [style, setStyle] = React.useState<React.CSSProperties>({
    height: '0px',
    visibility: 'hidden',
  });

  const onMouseMove = (e: MouseEvent) => {
    if (!isWithinBounds(e.offsetX, e.offsetY)) {
      onMouseOut();
      return;
    }

    setStyle({
      visibility: 'visible',
      height: `${barHeight}px`,
      ...xyToHighlightData(e.offsetX, e.offsetY),
    });
  };

  const onMouseOut = () => {
    setStyle({
      ...style,
      visibility: 'hidden',
    });
  };

  React.useEffect(() => {
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
  }, [canvasRef.current]);

  return (
    <div
      style={style}
      data-testid="flamegraph-highlight"
      className="flamegraph-highlight"
    />
  );
}
