import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import {
  fetchSingleView,
  setDateRange,
} from '@webapp/redux/reducers/continuous';
import TimelineChartWrapper from '@webapp/components/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';

function ContinuousSingleView() {
  const dispatch = useAppDispatch();

  const { from, until, query, refreshToken, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  const { singleView } = useAppSelector((state) => state.continuous);

  useEffect(() => {
    if (from && until && query && maxNodes) {
      const fetch = dispatch(fetchSingleView(null));
      return fetch.abort;
    }
    return () => null;
  }, [from, until, query, refreshToken, maxNodes, dispatch]);

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
            profile={singleView.profile}
            ExportData={
              <ExportData
                flamebearer={singleView.profile}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
                exportFlamegraphDotCom
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

  const getTimeline = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return {
          data: singleView.timeline,
        };
      }

      default: {
        return {
          data: undefined,
        };
      }
    }
  };

  return (
    <div>
      <div className="main-wrapper">
        <Toolbar />
        <TimelineChartWrapper
          data-testid="timeline-single"
          id="timeline-chart-single"
          timelineA={getTimeline()}
          onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
        />
        <Box>{flamegraphRenderer}</Box>
      </div>
    </div>
  );
}

export default ContinuousSingleView;
