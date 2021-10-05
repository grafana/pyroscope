import React from 'react';

export default function Highlight(props) {
  const { canvasRef, isWithinBounds, height, xyToHighlightData } = props;
  const [style, setStyle] = React.useState();

  const onMouseMove = (e) => {
    if (!isWithinBounds(e.offsetX, e.offsetY)) {
      onMouseOut();
      return;
    }

    setStyle({
      visibility: 'visible',
      height,
      ...xyToHighlightData(e.offsetX, e.offsetY),
    });
  };

  const onMouseOut = () => {
    setStyle({
      visibility: 'hidden',
    });
  };

  React.useEffect(() => {
    // use closure to "cache" the current canvas reference
    // so that when cleaning up, it points to a valid canvas
    // (otherwise it would be null)
    const canvasEl = canvasRef.current;
    if (!canvasEl) {
      return {};
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
