import React from "react";

import clsx from "clsx";

import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faBars } from "@fortawesome/free-solid-svg-icons/faBars";

import { jsPDF } from "jspdf";



function ExportData(props) {
  const [toggleMenu, setToggleMenu] = React.useState(false)

  const exportCanvasAsPNG = (event, paneType, mimeType) => {
    console.log('mimeType', mimeType);
    if (mimeType === 'pdf') {
      const canvas = document.querySelector('.flamegraph-canvas');
      const myImage = canvas.toDataURL("image/jpeg,1.0");
      // Adjust width and height
      const imgWidth = (canvas.width * 20) / 240;
      const imgHeight = (canvas.height * 20) / 240;
      // jspdf changes
      const pdf = new jsPDF('p', 'mm', 'a4');
      pdf.addImage(myImage, 'JPEG', 15, 2, imgWidth, imgHeight); // 2: 19
      pdf.save('Download.pdf');
      return
    }

    const canvasElement = document.querySelector('.flamegraph-canvas');
    const MIME_TYPE = `image/${mimeType}`;
    const imgURL = canvasElement.toDataURL();
    const dlLink = document.createElement('a');

    dlLink.download = `${Date.now()}file`;
    dlLink.href = imgURL;
    dlLink.dataset.downloadurl = [MIME_TYPE, dlLink.download, dlLink.href].join(':');

    document.body.appendChild(dlLink);
    dlLink.click();
    document.body.removeChild(dlLink);
  }

  const handleToggleMenu = event => {
    event.preventDefault()
    setToggleMenu(!toggleMenu)
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
            <div className='dropdown-menu-item' onClick={event => exportCanvasAsPNG(event, 'flame', 'pdf')}>PDF</div>
          </div>
          <div>
            <div className='dropdown-menu-item' onClick={event => exportCanvasAsPNG(event, 'flame', 'png')}>PNG</div>
          </div>
          <div className='dropdown-divider'></div>
          <div className='dropdown-header'>Export Table</div>
          <div>
            <div className='dropdown-menu-item' onClick={event => exportCanvasAsPNG(event, 'flame', 'pdf')}>PDF</div>
          </div>
          <div>
            <div className='dropdown-menu-item' onClick={event => exportCanvasAsPNG(event, 'flame', 'png')}>PNG</div>
          </div>
        </div>
      </div>
  );
}

export default ExportData;