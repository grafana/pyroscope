import React, { useState } from "react";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faBars } from "@fortawesome/free-solid-svg-icons/faBars";

import clsx from "clsx";
import { jsPDF } from "jspdf";

import 'jspdf-autotable'



function ExportData() {
  const [toggleMenu, setToggleMenu] = useState(false)

  const exportCanvas = mimeType => {
    if (mimeType === 'pdf') {
      const canvas = document.querySelector('.flamegraph-canvas');
      const myImage = canvas.toDataURL("image/jpeg,1.0");

      const pdf = new jsPDF("p", "mm", "a4");
      const pdfWidth = pdf.internal.pageSize.getWidth();

      // reduce canvas width to a fit on the pdf
      // could use a more robust validation/assignment
      // of the canvas width and heights
      let count = 0
      let canvasWidth = canvas.width
      if (canvasWidth > pdfWidth) {
        while (canvasWidth > pdfWidth) {
          count++
          canvasWidth = canvasWidth - pdfWidth
        }
        count++
      }

      // alignment
      const pdfXOffset = (pdfWidth - canvas.width/count) / 2
      const pdfYOffset = 30
      pdf.addImage(myImage, 'JPEG', pdfXOffset, pdfYOffset, (canvas.width / count), (canvas.height / count));

      const textXOffset = pdfXOffset
      const textYOffset = 10
      pdf.text(textXOffset, textYOffset, 'Flamegraph Visual')

      pdf.save(`flamegraph_visual_${formattedDate()}`);
      setToggleMenu(!toggleMenu);
      return
    }

    const canvasElement = document.querySelector('.flamegraph-canvas');
    const MIME_TYPE = `image/${mimeType}`;
    const imgURL = canvasElement.toDataURL();
    const dlLink = document.createElement('a');

    dlLink.download = `${Date.now()}`;
    dlLink.href = imgURL;
    dlLink.dataset.downloadurl = [MIME_TYPE, dlLink.download, dlLink.href].join(':');

    document.body.appendChild(dlLink);
    dlLink.click();
    document.body.removeChild(dlLink);
    setToggleMenu(!toggleMenu);
  }

  const exportTable = () => {
    const pdf = new jsPDF("p", "mm", "a4");
    pdf.autoTable({
      html: '.flamegraph-table',
      theme: 'grid',
    })
    pdf.save(`table_visual_${formattedDate()}`);
    setToggleMenu(!toggleMenu);
  }

  const formattedDate = () => {
    const cd = new Date();
    const d = cd.getDate() < 10 ? `0${cd.getDate()}` : `${cd.getDate()}`;
    const m = cd.getMonth() < 10 ? `0${cd.getMonth()}` : `${cd.getMonth()}`;
    const y = cd.getFullYear()
    return `${d}_${m}_${y}`
  }

  const handleToggleMenu = event => {
    event.preventDefault();
    setToggleMenu(!toggleMenu);
  }

  return (
      <div className='dropdown-container'>
        <button
          type="button"
          className="btn"
          onClick={handleToggleMenu}
        >
          <FontAwesomeIcon icon={faBars} />
        </button>

        <div className={clsx({'menu-show': toggleMenu, 'menu-hide': !toggleMenu})}>
          <div className='dropdown-header'>Export Flamegraph</div>
          <div>
            <div className='dropdown-menu-item' onClick={() => exportCanvas('pdf')}>PDF</div>
          </div>
          <div>
            <div className='dropdown-menu-item' onClick={() => exportCanvas('png')}>PNG</div>
          </div>
          <div className='dropdown-divider'/>
          <div className='dropdown-header'>Export Table</div>
          <div>
            <div className='dropdown-menu-item' onClick={() => exportTable('pdf')}>PDF</div>
          </div>
        </div>
      </div>
  );
}

export default ExportData;