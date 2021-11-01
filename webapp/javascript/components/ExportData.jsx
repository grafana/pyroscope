import React, { useState } from 'react';
import { connect } from 'react-redux';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';

import clsx from 'clsx';

function ExportData() {
  const [toggleMenu, setToggleMenu] = useState(false);

  const formattedDate = () => {
    const cd = new Date();
    const d = cd.getDate() < 10 ? `0${cd.getDate()}` : `${cd.getDate()}`;
    const m = cd.getMonth() < 10 ? `0${cd.getMonth()}` : `${cd.getMonth()}`;
    const y = cd.getFullYear();
    return `${d}_${m}_${y}`;
  };

  // export flamegraph canvas element
  const exportCanvas = (mimeType) => {
    const canvasElement = document.querySelector('.flamegraph-canvas');
    const MIME_TYPE = `image/${mimeType}`;
    const imgURL = canvasElement.toDataURL();
    const dlLink = document.createElement('a');

    dlLink.download = `flamegraph_visual_${formattedDate()}`;
    dlLink.href = imgURL;
    dlLink.dataset.downloadurl = [MIME_TYPE, dlLink.download, dlLink.href].join(
      ':'
    );

    document.body.appendChild(dlLink);
    dlLink.click();
    document.body.removeChild(dlLink);
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
        className={clsx({ 'menu-show': toggleMenu, 'menu-hide': !toggleMenu })}
      >
        <div className="dropdown-header">Export Flamegraph</div>
        <div>
          <button
            className="dropdown-menu-item"
            onClick={() => exportCanvas('png')}
            onKeyPress={() => exportCanvas('png')}
            type="button"
          >
            PNG
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
