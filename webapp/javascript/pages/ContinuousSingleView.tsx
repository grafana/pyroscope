import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import {
  fetchSingleView,
  selectQueries,
  setDateRange,
} from '@webapp/redux/reducers/continuous';
import useColorMode from '@webapp/hooks/colorMode.hook';
import TimelineChartWrapper from '@webapp/components/TimelineChart/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import TimelineTitle from '@webapp/components/TimelineTitle';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import PageTitle from '@webapp/components/PageTitle';
import { isExportToFlamegraphDotComEnabled } from '@webapp/util/features';
import { formatTitle } from './formatTitle';
import ContextMenu from '@webapp/components/TimelineChart/ContextMenu';
import { queryToAppName } from '@webapp/models/query';

function ContinuousSingleView() {
  const dispatch = useAppDispatch();
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();

  const { query } = useAppSelector(selectQueries);
  const { from, until, refreshToken, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  const { singleView } = useAppSelector((state) => state.continuous);

  useEffect(() => {
    if (from && until && query && maxNodes) {
      const fetchData = dispatch(fetchSingleView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
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

  const getTimeline = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return {
          data: singleView.timeline,
          annotations: singleView.annotations.annotations,
        };
      }

      default: {
        return {
          data: undefined,
          annotations: [],
        };
      }
    }
  };

  return (
    <div>
      <PageTitle title={formatTitle('Single', query)} />
      <div className="main-wrapper">
        <Toolbar />
        <Box>
          <TimelineChartWrapper
            timezone={offset === 0 ? 'utc' : 'browser'}
            data-testid="timeline-single"
            id="timeline-chart-single"
            timelineA={getTimeline()}
            annotations={getTimeline().annotations}
            ContextMenu={(props) => {
              const appName = queryToAppName(query);

              if (appName.isNothing) {
                return null;
              }

              return (
                <ContextMenu
                  appName={appName.value}
                  x={props.x}
                  y={props.y}
                  timestamp={props.timestamp}
                />
              );
            }}
            onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
            height="125px"
            title={
              <TimelineTitle titleKey={singleView?.profile?.metadata.units} />
            }
            selectionType="single"
          />
        </Box>
        <Box>{flamegraphRenderer}</Box>
      </div>
    </div>
  );
}

export default ContinuousSingleView;
