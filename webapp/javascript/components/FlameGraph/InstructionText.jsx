import React from "react";

function InstructionText(props) {
  const { viewType, viewSide } = props;
  const instructionsText =
    viewType === "double" || viewType === "diff"
      ? `Select ${viewSide} time range`
      : null;
  const instructionsClassName =
    viewType === "double" || viewType === "diff"
      ? `${viewSide}-instructions`
      : null;

  return (
    <div className={`${instructionsClassName}-wrapper`}>
      <span className={`${instructionsClassName}-text`}>
        {instructionsText}
      </span>
    </div>
  );
}

export default InstructionText;
