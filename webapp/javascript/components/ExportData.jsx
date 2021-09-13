import React, { useState } from "react";
import { connect } from "react-redux";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faBars } from "@fortawesome/free-solid-svg-icons/faBars";

import clsx from "clsx";
import { jsPDF as JSPDF } from "jspdf";
import "jspdf-autotable";

function ExportData(props) {
  const [toggleMenu, setToggleMenu] = useState(false);

  const formattedDate = () => {
    const cd = new Date();
    const d = cd.getDate() < 10 ? `0${cd.getDate()}` : `${cd.getDate()}`;
    const m = cd.getMonth() < 10 ? `0${cd.getMonth()}` : `${cd.getMonth()}`;
    const y = cd.getFullYear();
    return `${d}_${m}_${y}`;
  };

  const formatPdfTitle = () => {
    const { from, until } = props;

    return `${props.label} - from: ${from} - to ${until}`;
  };

  // export flamegraph canvas element
  const exportCanvas = (mimeType) => {
    if (mimeType === "pdf") {
      const canvas = document.querySelector(".flamegraph-canvas");
      const myImage = canvas.toDataURL("image/jpeg,1.0");

      const pdf = new JSPDF("p", "mm", "a4");
      const pdfWidth = pdf.internal.pageSize.getWidth();

      // reduce canvas width to a fit on the pdf
      // could use a more robust validation/assignment
      // of the canvas width and heights
      let count = 0;
      let canvasWidth = canvas.width;
      if (canvasWidth > pdfWidth) {
        while (canvasWidth > pdfWidth) {
          count += 1;
          canvasWidth -= pdfWidth;
        }
        count += 1;
      }

      // alignment
      const pdfXOffset = (pdfWidth - canvas.width / count) / 2;
      const pdfYOffset = 30;
      pdf.addImage(
        myImage,
        "JPEG",
        pdfXOffset,
        pdfYOffset,
        canvas.width / count,
        canvas.height / count
      );

      const textXOffset = pdfXOffset;
      const textYOffset = 10;
      pdf.text(textXOffset, textYOffset, formatPdfTitle());

      pdf.save(`flamegraph_visual_${formattedDate()}`);
      setToggleMenu(!toggleMenu);
      return;
    }

    const canvasElement = document.querySelector(".flamegraph-canvas");
    const MIME_TYPE = `image/${mimeType}`;
    const imgURL = canvasElement.toDataURL();
    const dlLink = document.createElement("a");

    dlLink.download = `flamegraph_visual_${formattedDate()}`;
    dlLink.href = imgURL;
    dlLink.dataset.downloadurl = [MIME_TYPE, dlLink.download, dlLink.href].join(
      ":"
    );

    document.body.appendChild(dlLink);
    dlLink.click();
    document.body.removeChild(dlLink);
    setToggleMenu(!toggleMenu);
  };

  // export the flamegraph table element
  const exportTable = () => {
    const pdf = new JSPDF("p", "mm", "a4");
    pdf.text(12, 7, formatPdfTitle());
    pdf.autoTable({
      html: ".flamegraph-table",
      theme: "grid",
    });
    pdf.save(`table_visual_${formattedDate()}`);
    setToggleMenu(!toggleMenu);
  };

  const handleToggleMenu = (event) => {
    event.preventDefault();
    setToggleMenu(!toggleMenu);
  };

  return (
    <div className="dropdown-container">
      <button type="button" className="btn" onClick={handleToggleMenu}>
        <FontAwesomeIcon icon={faBars} />
      </button>

      <div
        className={clsx({ "menu-show": toggleMenu, "menu-hide": !toggleMenu })}
      >
        <div className="dropdown-header">Export Flamegraph</div>
        <div>
          <button
            className="dropdown-menu-item"
            onClick={() => exportCanvas("pdf")}
            onKeyPress={() => exportCanvas("pdf")}
            type="button"
          >
            PDF
          </button>
        </div>
        <div>
          <button
            className="dropdown-menu-item"
            onClick={() => exportCanvas("png")}
            onKeyPress={() => exportCanvas("png")}
            type="button"
          >
            PNG
          </button>
        </div>
        <div className="dropdown-divider" />
        <div className="dropdown-header">Export Table</div>
        <div>
          <button
            className="dropdown-menu-item"
            onClick={() => exportTable("pdf")}
            onKeyPress={() => exportTable("pdf")}
            type="button"
          >
            PDF
          </button>
        </div>
      </div>
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state,
});

export default connect(mapStateToProps)(ExportData);
