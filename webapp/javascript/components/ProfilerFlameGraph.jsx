import React from "react";

function ProfilerFlameGraph() {
  return (
    <div className={clsx("pane", { hidden: this.state.view === "table" })}>
      <canvas
        className="flamegraph-canvas"
        height="0"
        ref={this.canvasRef}
        onClick={this.clickHandler}
        onMouseMove={this.mouseMoveHandler}
        onMouseOut={this.mouseOutHandler}
      />
    </div>
  );
}
