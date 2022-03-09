import React, { useEffect, useRef } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import {
  fetchSingleView,
  setDateRange,
} from '@pyroscope/redux/reducers/continuous';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import Toolbar from '../components/Toolbar';
import Footer from '../components/Footer';
import ExportData from '../components/ExportData';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';

function ContinuousSingleView() {
  const dispatch = useAppDispatch();

  const { from, until, query, refreshToken, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  const { singleView } = useAppSelector((state) => state.continuous);

  useEffect(() => {
    if (from && until && query && maxNodes) {
      dispatch(fetchSingleView());
    }
  }, [from, until, query, refreshToken, maxNodes]);

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
            viewType="single"
            display="both"
            rawFlamegraph={singleView.profile}
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

  const getTimelineData = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return singleView.timeline;
      }

      default:
        return undefined;
    }
  };

  console.log('singleViewProfile', singleView);
  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar />
        <TimelineChartWrapper
          data-testid="timeline-single"
          id="timeline-chart-single"
          viewSide="none"
          timeline={getTimelineData()}
          onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
        />
        <Box>{flamegraphRenderer}</Box>
      </div>
      <Footer />
    </div>
  );
}

export default ContinuousSingleView;
