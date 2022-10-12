import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import {
  fetchSingleView,
  selectQueries,
  setDateRange,
  selectAnnotationsOrDefault,
  addAnnotation,
} from '@webapp/redux/reducers/continuous';
import useColorMode from '@webapp/hooks/colorMode.hook';
import TimelineChartWrapper from '@webapp/components/TimelineChart/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import TimelineTitle from '@webapp/components/TimelineTitle';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import PageTitle from '@webapp/components/PageTitle';
import { ContextMenuProps } from '@webapp/components/TimelineChart/ContextMenu.plugin';
import { LoadingOverlay2 } from '@webapp/ui/LoadingOverlay';
import {
  isExportToFlamegraphDotComEnabled,
  isAnnotationsEnabled,
} from '@webapp/util/features';
import { formatTitle } from './formatTitle';
import ContextMenu from './continuous/contextMenu/ContextMenu';
import AddAnnotationMenuItem from './continuous/contextMenu/AddAnnotation.menuitem';

function ContinuousSingleView() {
  const dispatch = useAppDispatch();
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();

  const { query } = useAppSelector(selectQueries);
  const { from, until, refreshToken, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  const { singleView } = useAppSelector((state) => state.continuous);
  const annotations = useAppSelector(selectAnnotationsOrDefault);

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
        };
      }

      default: {
        return {
          data: undefined,
        };
      }
    }
  };

  const contextMenu = (props: ContextMenuProps) => {
    if (!isAnnotationsEnabled) {
      return null;
    }
    return (
      <ContextMenu position={props.click}>
        <AddAnnotationMenuItem
          container={props.containerEl}
          popoverAnchorPoint={{ x: props.click.pageX, y: props.click.pageY }}
          timestamp={props.timestamp}
          timezone={offset === 0 ? 'utc' : 'browser'}
          onCreateAnnotation={(content) => {
            dispatch(
              addAnnotation({
                appName: query,
                timestamp: props.timestamp,
                content,
              })
            );
          }}
        />
      </ContextMenu>
    );
  };
  return (
    <div>
      <PageTitle title={formatTitle('Single', query)} />
      <div className="main-wrapper">
        <Toolbar />

        <Box>
          <LoadingOverlay2 active={singleView.type === 'reloading'}>
            <TimelineChartWrapper
              timezone={offset === 0 ? 'utc' : 'browser'}
              data-testid="timeline-single"
              id="timeline-chart-single"
              timelineA={getTimeline()}
              onSelect={(from, until) =>
                dispatch(setDateRange({ from, until }))
              }
              height="125px"
              title={
                <TimelineTitle titleKey={singleView?.profile?.metadata.units} />
              }
              annotations={annotations}
              selectionType="single"
              ContextMenu={contextMenu}
            />
          </LoadingOverlay2>
        </Box>
        <Box>
          <LoadingOverlay2
            spinnerPosition="baseline"
            active={singleView.type === 'reloading'}
          >
            {flamegraphRenderer}
          </LoadingOverlay2>
        </Box>
      </div>
    </div>
  );
}

export default ContinuousSingleView;
