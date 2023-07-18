/* eslint-disable react/destructuring-assignment */
import React, { useState } from 'react';
import { format } from 'date-fns';
import OutsideClickHandler from 'react-outside-click-handler';
import { Tooltip } from '@pyroscope/webapp/javascript/ui/Tooltip';
import Button from '@webapp/ui/Button';
import { faShareSquare } from '@fortawesome/free-solid-svg-icons/faShareSquare';
import { createBiggestInterval } from '@webapp/util/timerange';
import { convertPresetsToDate, formatAsOBject } from '@webapp/util/formatDate';
import { Profile } from '@pyroscope/models/src';
import { ContinuousState } from '@pyroscope/webapp/javascript/redux/reducers/continuous/state';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { isRouteActive, ROUTES } from '@phlare/pages/routes';
import { useLocation } from 'react-router-dom';
import showModalWithInput from '@pyroscope/webapp/javascript/components/Modals/ModalWithInput';
import styles from '@pyroscope/webapp/javascript/components/ExportData.module.scss';
import { downloadWithOrgID } from '@webapp/services/base';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { Message, Field } from 'protobufjs/light';
import handleError from '@webapp/util/handleError';

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

export class PprofRequest extends Message<PprofRequest> {
  constructor(
    profile_typeID: string,
    label_selector: string,
    start: number,
    end: number
  ) {
    super();
    this.profile_typeID = profile_typeID;
    this.label_selector = label_selector;
    this.start = start;
    this.end = end;
  }
  @Field.d(1, 'string')
  profile_typeID: string;
  @Field.d(2, 'string')
  label_selector: string;
  @Field.d(3, 'int64')
  start: number;
  @Field.d(4, 'int64')
  end: number;
}

type ExportDataProps = exportPprof &
  exportHTML &
  exportFlamegraphDotCom &
  exportPNG &
  exportJSON;

function biggestTimeRangeInUnixMs(state: ContinuousState) {
  return createBiggestInterval({
    from: [state.from, state.leftFrom, state.rightFrom]
      .map(formatAsOBject)
      .map((d) => d.valueOf()),
    until: [state.until, state.leftUntil, state.leftUntil]
      .map(formatAsOBject)
      .map((d) => d.valueOf()),
  });
}

function buildPprofQuery(state: ContinuousState) {
  const { from, until } = biggestTimeRangeInUnixMs(state);
  const labelsIndex = state.query.indexOf('{');
  const profileTypeID = state.query.substring(0, labelsIndex);
  const label_selector = state.query.substring(labelsIndex);
  const message = new PprofRequest(profileTypeID, label_selector, from, until);
  return PprofRequest.encode(message).finish();
}

function ExportData(props: ExportDataProps) {
  const { exportJSON = false } = props;
  let exportPprof = props.exportPprof;
  let exportFlamegraphDotCom = false; // todo: add support for flamegraph.com
  let exportPNG = true;
  let exportHTML = false;
  const { pathname } = useLocation();
  const dispatch = useAppDispatch();
  const pprofQuery = useAppSelector((state: { continuous: ContinuousState }) =>
    buildPprofQuery(state.continuous)
  );

  if (
    isRouteActive(pathname, ROUTES.COMPARISON_DIFF_VIEW) ||
    isRouteActive(pathname, ROUTES.COMPARISON_VIEW)
  ) {
    exportPprof = false;
  }
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
      if (!customExportName) {
        return;
      }

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
    if (!exportFlamegraphDotCom) {
      return;
    }

    const { flamebearer } = props;

    const defaultExportName = getFilename(
      flamebearer.metadata.appName,
      flamebearer.metadata.startTime,
      flamebearer.metadata.endTime
    );
    // get user input from modal
    const customExportName = await getCustomExportName(defaultExportName);
    // return if user cancels the modal
    if (!customExportName) {
      return;
    }
    // todo CORS
    const response = await fetch('https://flamegraph.com/upload/v1', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
      },
      body: JSON.stringify({
        fileTypeData: {
          units: flamebearer.metadata.units,
          spyName: flamebearer.metadata.spyName,
        },
        name: customExportName,
        profile: btoa(JSON.stringify(flamebearer)),
        type: 'json',
      }),
    });
    const data = await response.json();
    console.log(data);
    // props.exportFlamegraphDotComFn(customExportName).then((url) => {
    //   // there has been an error which should've been handled
    //   // so we just ignore it
    //   if (!url) {
    //     return;
    //   }

    //   const dlLink = document.createElement('a');
    //   dlLink.target = '_blank';
    //   dlLink.href = url;

    //   document.body.appendChild(dlLink);
    //   dlLink.click();
    //   document.body.removeChild(dlLink);
    // });
  };

  const downloadPNG = async () => {
    if (exportPNG) {
      const { flamebearer } = props;

      const defaultExportName = getFilename(
        flamebearer.metadata.appName,
        flamebearer.metadata.startTime,
        flamebearer.metadata.endTime
      );
      // get user input from modal
      const customExportName = await getCustomExportName(defaultExportName);
      // return if user cancels the modal
      if (!customExportName) {
        return;
      }

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

  const downloadPprof = async function () {
    if (!exportPprof) {
      return;
    }

    if (props.exportPprof) {
      // get user input from modal
      const customExportName = await getCustomExportName('profile.pb.gz');
      // return if user cancels the modal
      if (!customExportName) {
        return;
      }
      let response = await downloadWithOrgID(
        '/querier.v1.QuerierService/SelectMergeProfile',
        {
          headers: {
            'content-type': 'application/proto',
          },
          method: 'POST',
          body: pprofQuery,
        }
      );
      if (response.isErr) {
        handleError(dispatch, 'Failed to export to pprof', response.error);
        return null;
      }
      let data = await new Response(
        response.value.body?.pipeThrough(new CompressionStream('gzip'))
      ).blob();
      let element = document.createElement('a');
      element.setAttribute('href', window.URL.createObjectURL(data));
      element.setAttribute('download', customExportName);
      element.style.display = 'none';
      document.body.appendChild(element);

      element.click();
    }
  };

  const downloadHTML = async function () {};

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
