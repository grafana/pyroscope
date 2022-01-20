/* eslint-disable react/destructuring-assignment */
import React, { useState } from 'react';

import Button from '@ui/Button';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { buildRenderURL } from '@utils/updateRequests';
import { dateForExportFilename } from '@utils/formatDate';

import clsx from 'clsx';
import { RawFlamebearerProfile } from '@models/flamebearer';

// These are modeled individually since each condition may have different values
// For example, a exportPprof: true may accept a custom export function
// For cases like grafana
type exportJSON =
  | {
      exportJSON: true;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportJSON?: false };

type exportPprof =
  | {
      exportPprof: true;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportPprof?: false };

type exportHTML =
  | {
      exportHTML: true;
      fetchUrlFunc?: () => string;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportHTML?: false };

type exportPNG =
  | {
      exportPNG: true;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportPNG?: false };

type ExportDataProps = exportPprof & exportJSON & exportHTML & exportPNG;

function ExportData(props: ExportDataProps) {
  const {
    exportPprof = false,
    exportJSON = false,
    exportPNG = false,
    exportHTML = false,
  } = props;
  if (!exportPNG && !exportJSON && !exportPprof && !exportHTML) {
    throw new Error('At least one export button should be enabled');
  }

  const [toggleMenu, setToggleMenu] = useState(false);

  const downloadJSON = () => {
    if (!props.exportJSON) {
      return;
    }

    // TODO additional check this won't be needed once we use strictNullChecks
    if (props.exportJSON) {
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

  const downloadPNG = () => {
    if (props.exportPNG) {
      const { flamebearer } = props;
      const mimeType = 'png';
      // TODO use ref
      // this won't work for comparison side by side
      const canvasElement = document.querySelector(
        '.flamegraph-canvas'
      ) as HTMLCanvasElement;
      const MIME_TYPE = `image/${mimeType}`;
      const imgURL = canvasElement.toDataURL();
      const dlLink = document.createElement('a');
      const dateForFilename = dateForExportFilename(
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );

      dlLink.download = `${flamebearer.metadata.appName}_${dateForFilename}.${mimeType}`;
      dlLink.href = imgURL;
      dlLink.dataset.downloadurl = [
        MIME_TYPE,
        dlLink.download,
        dlLink.href,
      ].join(':');

      document.body.appendChild(dlLink);
      dlLink.click();
      document.body.removeChild(dlLink);
      setToggleMenu(!toggleMenu);
    }
  };

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

  const downloadHTML = function () {
    if (props.exportHTML) {
      const { flamebearer } = props;

      const url =
        typeof props.fetchUrlFunc === 'function'
          ? props.fetchUrlFunc()
          : buildRenderURL({
              from: flamebearer.metadata.startTime,
              until: flamebearer.metadata.endTime,
              query: flamebearer.metadata.query,
              maxNodes: flamebearer.metadata.maxNodes,
            });
      const urlWithFormat = `${url}&format=html`;

      const dateForFilename = dateForExportFilename(
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );
      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('href', urlWithFormat);
      downloadAnchorNode.setAttribute(
        'download',
        `${flamebearer.metadata.appName}_${dateForFilename}.html`
      );

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
        {exportPNG && (
          <button
            className="dropdown-menu-item"
            onClick={() => downloadPNG()}
            onKeyPress={() => downloadPNG()}
            type="button"
          >
            Png
          </button>
        )}
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
        {exportHTML && (
          <button
            className="dropdown-menu-item"
            type="button"
            onClick={() => downloadHTML()}
          >
            {' '}
            Html
          </button>
        )}
      </div>
    </div>
  );
}

export default ExportData;
