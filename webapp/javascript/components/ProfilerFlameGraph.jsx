import React from "react";
import clsx from "clsx";

export default function ProfilerFlameGraph({
  view,
  canvasRef,
  clickHandler,
  mouseMoveHandler,
  mouseOutHandler,
}) {
  console.log(view);
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
