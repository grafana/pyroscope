import React from "react";

export default function ProfilerFlameGraph({
  view,
  canvasRef,
  clickHandler,
  mouseMoveHandler,
  mouseOutHandler,
}) {
  return (
    <div className={clsx("pane", { hidden: view === "table" })}>
      <canvas
        className="flamegraph-canvas"
        height="0"
        ref={canvasRef}
        onClick={clickHandler}
        onMouseMove={mouseMoveHandler}
        onMouseOut={mouseOutHandler}
      />
    </div>
  );
}
