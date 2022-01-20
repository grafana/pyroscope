/* eslint-disable react/destructuring-assignment */
import React, { useState } from 'react';

import Button from '@ui/Button';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { buildRenderURL } from '@utils/updateRequests';

import clsx from 'clsx';
import { RawFlamebearerProfile } from '@models/flamebearer';

type exportJSON =
  | {
      // if we export JSON, we absolutely need
      // the raw flamebearer
      exportJSON: true;
      exportJSONFn?: () => void;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportJSON?: false };

type exportPprof =
  | {
      // if we are expoting json, we also need the flamebearer
      // TODO not really?
      exportPprof: true;
      exportPProfFunc?: () => void;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportPprof?: false };

type ExportDataProps = exportPprof &
  exportJSON & {
    exportPNG?: boolean;
  };

function ExportData(props: ExportDataProps) {
  const { exportPprof = false, exportJSON = false, exportPNG = false } = props;
  if (!exportPNG && !exportJSON && !exportPprof) {
    throw new Error('At least one export button should be enabled');
  }

  const [toggleMenu, setToggleMenu] = useState(false);

  const downloadJSON = () => {
    if (!props.exportJSON) {
      return;
    }

    // TODO additional check this won't be needed once we use strictNullChecks
    if (props.exportJSON) {
      console.log('generating');
      const exportObj = props.flamebearer;
      const exportName = 'pyroscope_export';

      const dataStr = `data:text/json;charset=utf-8,${encodeURIComponent(
        JSON.stringify(exportObj)
      )}`;
      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('data-testid', 'export-json');
      downloadAnchorNode.setAttribute('href', dataStr);
      downloadAnchorNode.setAttribute('download', `${exportName}.json`);
      document.body.appendChild(downloadAnchorNode); // required for firefox
      downloadAnchorNode.click();
      downloadAnchorNode.remove();
    }
  };

  //
  //  const formattedDate = () => {
  //    const cd = new Date();
  //    const d = cd.getDate() < 10 ? `0${cd.getDate()}` : `${cd.getDate()}`;
  //    const m = cd.getMonth() < 10 ? `0${cd.getMonth()}` : `${cd.getMonth()}`;
  //    const y = cd.getFullYear();
  //    return `${d}_${m}_${y}`;
  //  };
  //
  //  // export flamegraph canvas element
  //  const exportCanvas = (mimeType: 'png') => {
  //    // TODO use ref
  //    const canvasElement = document.querySelector(
  //      '.flamegraph-canvas'
  //    ) as HTMLCanvasElement;
  //    const MIME_TYPE = `image/${mimeType}`;
  //    const imgURL = canvasElement.toDataURL();
  //    const dlLink = document.createElement('a');
  //
  //    dlLink.download = `flamegraph_visual_${formattedDate()}`;
  //    dlLink.href = imgURL;
  //    dlLink.dataset.downloadurl = [MIME_TYPE, dlLink.download, dlLink.href].join(
  //      ':'
  //    );
  //
  //    document.body.appendChild(dlLink);
  //    dlLink.click();
  //    document.body.removeChild(dlLink);
  //    setToggleMenu(!toggleMenu);
  //  };
  //
  const handleToggleMenu = (event: React.MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    setToggleMenu(!toggleMenu);
  };

  const downloadPprof = function () {
    if (!props.exportPprof) {
      return;
    }

    if (props.exportPprof) {
      const { flamebearer } = props;

      // TODO
      // This build url won't work in the following cases:
      // * absence of a public server (grafana, standalone)
      // * diff mode
      const url = `${buildRenderURL({
        from: flamebearer.metadata.startTime,
        until: flamebearer.metadata.endTime,
        query: flamebearer.metadata.query,
        maxNodes: flamebearer.metadata.maxNodes,
      })}&format=pprof`;

      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('href', url);
      document.body.appendChild(downloadAnchorNode); // required for firefox
      downloadAnchorNode.click();
      downloadAnchorNode.remove();
    }
  };

  return (
    <div className="dropdown-container">
      <Button icon={faBars} onClick={handleToggleMenu} />
      <div
        className={clsx({ 'menu-show': toggleMenu, 'menu-hide': !toggleMenu })}
      >
        {exportJSON && (
          <button
            className="dropdown-menu-item"
            type="button"
            onClick={() => downloadJSON()}
          >
            JSON
          </button>
        )}
        {exportPprof && (
          <button
            className="dropdown-menu-item"
            type="button"
            onClick={() => downloadPprof()}
          >
            pprof
          </button>
        )}
      </div>
    </div>
  );
  //  return (
  //    <div className="dropdown-container">
  //      <Button icon={faBars} onClick={handleToggleMenu} />
  //
  //      <div
  //        className={clsx({ 'menu-show': toggleMenu, 'menu-hide': !toggleMenu })}
  //      >
  //        <div className="dropdown-header">Export Flamegraph</div>
  //        <div>
  //          {exportPNG && (
  //            <button
  //              className="dropdown-menu-item"
  //              onClick={() => exportCanvas('png')}
  //              onKeyPress={() => exportCanvas('png')}
  //              type="button"
  //            >
  //              PNG
  //            </button>
  //          )}
  //          {exportJSON && (
  //            <button
  //              className="dropdown-menu-item"
  //              type="button"
  //              onClick={() =>
  //                downloadFlamebearer(flamebearer, 'pyroscope_export')
  //              }
  //            >
  //              JSON
  //            </button>
  //          )}
  //
  //          {exportPprof && (
  //            <button
  //              className="dropdown-menu-item"
  //              type="button"
  //              onClick={() => exportPProfFunc(flamebearer)}
  //            >
  //              pprof
  //            </button>
  //          )}
  //        </div>
  //      </div>
  //    </div>
  //    );
}

export default ExportData;
