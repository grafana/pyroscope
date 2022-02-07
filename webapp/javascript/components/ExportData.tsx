/* eslint-disable react/destructuring-assignment */
import React, { useState } from 'react';
import { useDispatch } from 'react-redux';

import Button from '@ui/Button';
import { faBars } from '@fortawesome/free-solid-svg-icons/faBars';
import { buildRenderURL } from '@utils/updateRequests';
import { dateForExportFilename } from '@utils/formatDate';

import clsx from 'clsx';
import { RawFlamebearerProfile } from '@models/flamebearer';
import { addNotification } from '../redux/reducers/notifications';

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

type exportFlamegraphDotCom =
  | {
      exportFlamegraphDotCom: true;
      fetchUrlFunc?: () => string;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportFlamegraphDotCom?: false };

type exportPNG =
  | {
      exportPNG: true;
      flamebearer: RawFlamebearerProfile;
    }
  | { exportPNG?: false };

type ExportDataProps = exportPprof &
  exportJSON &
  exportHTML &
  exportFlamegraphDotCom &
  exportPNG;

// handleResponse retrieves the JSON data on success or raises an ResponseNotOKError otherwise
// TODO: dedup this with the one in actions.js
function handleResponse(response) {
  console.log(response, response.ok);
  if (response.ok) {
    return response.json();
  }
  return response.text().then((text) => {
    throw new Error(`Bad Response with code ${response.status}: ${text}`);
  });
}

function ExportData(props: ExportDataProps) {
  const {
    exportPprof = false,
    exportJSON = false,
    exportPNG = false,
    exportHTML = false,
    exportFlamegraphDotCom = false,
  } = props;
  if (
    !exportPNG &&
    !exportJSON &&
    !exportPprof &&
    !exportHTML &&
    !exportFlamegraphDotCom
  ) {
    throw new Error('At least one export button should be enabled');
  }

  const dispatch = useDispatch();

  const [toggleMenu, setToggleMenu] = useState(false);

  const downloadJSON = () => {
    if (!props.exportJSON) {
      return;
    }

    // TODO additional check this won't be needed once we use strictNullChecks
    if (props.exportJSON) {
      const { flamebearer } = props;
      const filename = `${getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      )}.json`;

      const dataStr = `data:text/json;charset=utf-8,${encodeURIComponent(
        JSON.stringify(flamebearer)
      )}`;
      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('href', dataStr);
      downloadAnchorNode.setAttribute('download', filename);
      document.body.appendChild(downloadAnchorNode); // required for firefox
      downloadAnchorNode.click();
      downloadAnchorNode.remove();
    }
  };

  const downloadFlamegraphDotCom = () => {
    if (!props.exportFlamegraphDotCom) {
      return;
    }

    // TODO additional check this won't be needed once we use strictNullChecks
    if (props.exportFlamegraphDotCom) {
      const { flamebearer } = props;

      const filename = `${getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      )}.png`;

      // we don't upload to flamegraph.com directly due to potential CORS issues
      fetch('/export', {
        method: 'POST',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          name: filename,
          profile: btoa(JSON.stringify(flamebearer)),
          type: 'application/json',
        }),
      })
        .then((response) => handleResponse(response))
        .then((json) => {
          const dlLink = document.createElement('a');
          dlLink.target = '_blank';
          dlLink.href = json.url;

          document.body.appendChild(dlLink);
          dlLink.click();
          document.body.removeChild(dlLink);
        })
        .catch((e) => {
          dispatch(
            addNotification({
              title: 'Request Failed',
              message: e.message,
              type: 'danger',
            })
          );
        })
        .finally();
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
      const filename = `${getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      )}.png`;

      dlLink.download = filename;
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

      if (
        !flamebearer.metadata.startTime ||
        !flamebearer.metadata.endTime ||
        !flamebearer.metadata.query ||
        !flamebearer.metadata.maxNodes
      ) {
        throw new Error(
          'Missing one of the required parameters "flamebearer.metadata.startTime", "flamebearer.metadata.endTime", "flamebearer.metadata.query", "flamebearer.metadata.maxNodes"'
        );
      }

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

      if (
        !flamebearer.metadata.startTime ||
        !flamebearer.metadata.endTime ||
        !flamebearer.metadata.query ||
        !flamebearer.metadata.maxNodes
      ) {
        throw new Error(
          'Missing one of the required parameters "flamebearer.metadata.startTime", "flamebearer.metadata.endTime", "flamebearer.metadata.query", "flamebearer.metadata.maxNodes"'
        );
      }

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
      const filename = `${getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      )}.html`;

      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('href', urlWithFormat);
      downloadAnchorNode.setAttribute('download', filename);
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
            png
          </button>
        )}
        {exportJSON && (
          <button
            className="dropdown-menu-item"
            type="button"
            onClick={() => downloadJSON()}
          >
            json
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
            html
          </button>
        )}
        {exportFlamegraphDotCom && (
          <button
            className="dropdown-menu-item"
            type="button"
            onClick={() => downloadFlamegraphDotCom()}
          >
            {' '}
            flamegraph.com
          </button>
        )}
      </div>
    </div>
  );
}

export function getFilename(
  appName?: string,
  startTime?: number,
  endTime?: number
) {
  //  const appname = flamebearer.metadata.appName;
  let date = '';

  if (startTime && endTime) {
    date = dateForExportFilename(startTime.toString(), endTime.toString());
  }

  // both name and date are available
  if (appName && date) {
    return [appName, date].join('_');
  }

  // only fullname
  if (appName) {
    return appName;
  }

  // only date
  if (date) {
    return ['flamegraph', date].join('_');
  }

  // nothing is available, use a generic name
  return `flamegraph`;
}

export default ExportData;
