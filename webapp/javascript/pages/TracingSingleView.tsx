import React, { useEffect } from 'react';
import 'react-dom';

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

function TracingSingleView() {
  const dispatch = useAppDispatch();
  const { colorMode } = useColorMode();

  const { queryID, refreshToken, maxNodes } = useAppSelector(
    (state) => state.tracing
  );

  const { singleView } = useAppSelector((state) => state.tracing);

  useEffect(() => {
    console.log(queryID, maxNodes);
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

  return (
    <div>
      <PageTitle title={formatTitle('Tracing')} />
      <div className="main-wrapper">
        <Box>{flamegraphRenderer}</Box>
      </div>
    </div>
  );
}

export default TracingSingleView;
