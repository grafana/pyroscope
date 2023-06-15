/* eslint-disable react/destructuring-assignment */
import React, { useState } from 'react';
import { format } from 'date-fns';
import OutsideClickHandler from 'react-outside-click-handler';
import { Tooltip } from '@pyroscope/webapp/javascript/ui/Tooltip';
import Button from '@webapp/ui/Button';
import { faShareSquare } from '@fortawesome/free-solid-svg-icons/faShareSquare';
import { buildRenderURL } from '@webapp/util/updateRequests';
import { convertPresetsToDate } from '@webapp/util/formatDate';
import { Profile } from '@pyroscope/models/src';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { basename } from '@webapp/util/baseurl';
import showModalWithInput from './Modals/ModalWithInput';
import styles from './ExportData.module.scss';

// These are modeled individually since each condition may have different values
// For example, a exportPprof: true may accept a custom export function
// For cases like grafana
type exportJSON = {
  exportJSON?: boolean;
  flamebearer: Profile;
};

type exportPprof = {
  exportPprof?: boolean;
  flamebearer: Profile;
};

type exportHTML = {
  exportHTML?: boolean;
  fetchUrlFunc?: () => string;
  flamebearer: Profile;
};

type exportFlamegraphDotCom = {
  exportFlamegraphDotCom?: boolean;
  exportFlamegraphDotComFn?: (name?: string) => Promise<string | null>;
  flamebearer: Profile;
};

type exportPNG = {
  exportPNG?: boolean;
  flamebearer: Profile;
};

type ExportDataProps = exportPprof &
  exportHTML &
  exportFlamegraphDotCom &
  exportPNG &
  exportJSON;

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

  const [toggleMenu, setToggleMenu] = useState(false);

  const downloadJSON = async () => {
    if (!props.exportJSON) {
      return;
    }

    // TODO additional check this won't be needed once we use strictNullChecks
    if (props.exportJSON) {
      const { flamebearer } = props;

      const defaultExportName = getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );
      // get user input from modal
      const customExportName = await getCustomExportName(defaultExportName);
      // return if user cancels the modal
      if (!customExportName) return;

      const filename = `${customExportName}.json`;

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

  const downloadFlamegraphDotCom = async () => {
    if (!props.exportFlamegraphDotCom) {
      return;
    }

    // TODO additional check this won't be needed once we use strictNullChecks
    if (props.exportFlamegraphDotCom && props.exportFlamegraphDotComFn) {
      const { flamebearer } = props;

      const defaultExportName = getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );
      // get user input from modal
      const customExportName = await getCustomExportName(defaultExportName);
      // return if user cancels the modal
      if (!customExportName) return;

      props.exportFlamegraphDotComFn(customExportName).then((url) => {
        // there has been an error which should've been handled
        // so we just ignore it
        if (!url) {
          return;
        }

        const dlLink = document.createElement('a');
        dlLink.target = '_blank';
        dlLink.href = url;

        document.body.appendChild(dlLink);
        dlLink.click();
        document.body.removeChild(dlLink);
      });
    }
  };

  const downloadPNG = async () => {
    if (props.exportPNG) {
      const { flamebearer } = props;

      const defaultExportName = getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );
      // get user input from modal
      const customExportName = await getCustomExportName(defaultExportName);
      // return if user cancels the modal
      if (!customExportName) return;

      const filename = `${customExportName}.png`;

      const mimeType = 'png';
      // TODO use ref
      // this won't work for comparison side by side
      const canvasElement = document.querySelector(
        '.flamegraph-canvas'
      ) as HTMLCanvasElement;
      const MIME_TYPE = `image/${mimeType}`;
      const imgURL = canvasElement.toDataURL();
      const dlLink = document.createElement('a');

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
      let url = `${buildRenderURL({
        from: flamebearer.metadata.startTime.toString(),
        until: flamebearer.metadata.endTime.toString(),
        query: flamebearer.metadata.query,
        maxNodes: flamebearer.metadata.maxNodes,
      })}&format=pprof`;
      url = baseURLCompatible(url);
      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('href', url);
      document.body.appendChild(downloadAnchorNode); // required for firefox
      downloadAnchorNode.click();
      downloadAnchorNode.remove();
      setToggleMenu(false);
    }
  };

  const downloadHTML = async function () {
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
              from: flamebearer.metadata.startTime.toString(),
              until: flamebearer.metadata.endTime.toString(),
              query: flamebearer.metadata.query,
              maxNodes: flamebearer.metadata.maxNodes,
            });
      let urlWithFormat = `${url}&format=html`;
      urlWithFormat = baseURLCompatible(urlWithFormat);
      const defaultExportName = getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );
      // get user input from modal
      const customExportName = await getCustomExportName(defaultExportName);
      // return if user cancels the modal
      if (!customExportName) return;

      const filename = `${customExportName}.html`;

      const downloadAnchorNode = document.createElement('a');
      downloadAnchorNode.setAttribute('href', urlWithFormat);
      downloadAnchorNode.setAttribute('download', filename);
      document.body.appendChild(downloadAnchorNode); // required for firefox
      downloadAnchorNode.click();
      downloadAnchorNode.remove();
    }
  };

  async function getCustomExportName(defaultExportName: string) {
    return showModalWithInput({
      title: 'Enter export name',
      confirmButtonText: 'Export',
      input: 'text',
      inputValue: defaultExportName,
      inputPlaceholder: 'Export name',
      type: 'normal',
      validationMessage: 'Name must not be empty',
      onConfirm: (value: ShamefulAny) => value,
    });
  }

  return (
    <div className={styles.dropdownContainer}>
      <OutsideClickHandler onOutsideClick={() => setToggleMenu(false)}>
        <Tooltip placement="top" title="Export Data">
          <Button
            className={styles.toggleMenuButton}
            onClick={handleToggleMenu}
          >
            <FontAwesomeIcon icon={faShareSquare} />
          </Button>
        </Tooltip>
        <div className={toggleMenu ? styles.menuShow : styles.menuHide}>
          {exportPNG && (
            <button
              className={styles.dropdownMenuItem}
              onClick={downloadPNG}
              onKeyPress={downloadPNG}
              type="button"
            >
              png
            </button>
          )}
          {exportJSON && (
            <button
              className={styles.dropdownMenuItem}
              type="button"
              onClick={downloadJSON}
            >
              json
            </button>
          )}
          {exportPprof && (
            <button
              className={styles.dropdownMenuItem}
              type="button"
              onClick={downloadPprof}
            >
              pprof
            </button>
          )}
          {exportHTML && (
            <button
              className={styles.dropdownMenuItem}
              type="button"
              onClick={downloadHTML}
            >
              {' '}
              html
            </button>
          )}
          {exportFlamegraphDotCom && (
            <button
              className={styles.dropdownMenuItem}
              type="button"
              onClick={downloadFlamegraphDotCom}
            >
              {' '}
              flamegraph.com
            </button>
          )}
        </div>
      </OutsideClickHandler>
    </div>
  );
}

function baseURLCompatible(url: string) {
  const base = basename();
  if (base) {
    url = `${base}${url}`;
  }
  return url;
}

const dateFormat = 'yyyy-MM-dd_HHmm';

function dateForExportFilename(from: string, until: string) {
  let start = new Date(Math.round(parseInt(from, 10) * 1000));
  let end = new Date(Math.round(parseInt(until, 10) * 1000));

  if (/^now-/.test(from) && until === 'now') {
    const { _from } = convertPresetsToDate(from);

    start = new Date(Math.round(parseInt(_from.toString(), 10) * 1000));
    end = new Date();
  }

  return `${format(start, dateFormat)}-to-${format(end, dateFormat)}`;
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
