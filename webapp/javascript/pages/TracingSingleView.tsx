import React, { useEffect } from 'react';
import 'react-dom';
import { format } from 'date-fns';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import { fetchSingleView } from '@webapp/redux/reducers/tracing';
import useColorMode from '@webapp/hooks/colorMode.hook';
import ExportData from '@webapp/components/ExportData';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import PageTitle from '@webapp/components/PageTitle';
import { isExportToFlamegraphDotComEnabled } from '@webapp/util/features';
import { formatTitle } from './formatTitle';

import styles from './TracingSingleView.module.scss';

function formatTime(t: string | undefined): string {
  return format(new Date(1000 * parseInt(t || '0', 10)), 'yyyy-MM-dd HH:mm:ss');
}

function TracingSingleView() {
  const dispatch = useAppDispatch();
  const { colorMode } = useColorMode();

  const { queryID, refreshToken, maxNodes, singleView } = useAppSelector(
    (state) => state.tracing
  );

  useEffect(() => {
    if (queryID && maxNodes) {
      const fetchData = dispatch(fetchSingleView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [queryID, refreshToken, maxNodes]);

  const getRaw = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return singleView.profile;
      }

      default: {
        return undefined;
      }
    }
  };
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(getRaw());

  const flamegraphRenderer = (() => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return (
          <FlamegraphRenderer
            showCredit={false}
            profile={singleView.profile}
            colorMode={colorMode}
            ExportData={
              <ExportData
                flamebearer={singleView.profile}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
                exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
                exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
              />
            }
          />
        );
      }

      default: {
        return 'Loading';
      }
    }
  })();

  const header = singleView.mergeMetadata
    ? (function (mm) {
        const { appName, startTime, endTime, profilesLength } = mm;
        return (
          <>
            <div>
              <strong>App Name:</strong> <span>{appName}</span>
            </div>
            <div>
              <strong>Start Time:</strong> <span>{formatTime(startTime)}</span>
            </div>
            <div>
              <strong>End Time:</strong> <span>{formatTime(endTime)}</span>
            </div>
            <div>
              <strong>Number of Profiles merged:</strong>{' '}
              <span>{profilesLength}</span>
            </div>
          </>
        );
      })(singleView.mergeMetadata)
    : null;

  return (
    <div>
      <PageTitle title={formatTitle('Tracing')} />
      <div className="main-wrapper">
        <Box className={styles.header}>{header}</Box>
        <Box>{flamegraphRenderer}</Box>
      </div>
    </div>
  );
}

export default TracingSingleView;
