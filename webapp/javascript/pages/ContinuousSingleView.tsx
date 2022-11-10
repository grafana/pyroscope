import React, { useEffect } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import {
  fetchSingleView,
  setQuery,
  selectQueries,
  setDateRange,
  selectAnnotationsOrDefault,
  addAnnotation,
  actions,
  fetchTagValues,
} from '@webapp/redux/reducers/continuous';
import useColorMode from '@webapp/hooks/colorMode.hook';
import TimelineChartWrapper from '@webapp/components/TimelineChart/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import ExportData from '@webapp/components/ExportData';
import ChartTitle from '@webapp/components/ChartTitle';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import TagsBar from '@webapp/components/TagsBar';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import PageTitle from '@webapp/components/PageTitle';
import { ContextMenuProps } from '@webapp/components/TimelineChart/ContextMenu.plugin';
import { LoadingOverlay } from '@webapp/ui/LoadingOverlay';
import {
  isExportToFlamegraphDotComEnabled,
  isAnnotationsEnabled,
} from '@webapp/util/features';
import useTags from '@webapp/hooks/tags.hook';
import { formatTitle } from './formatTitle';
import ContextMenu from './continuous/contextMenu/ContextMenu';
import AddAnnotationMenuItem from './continuous/contextMenu/AddAnnotation.menuitem';
import { isLoadingOrReloading } from './loading';

function ContinuousSingleView() {
  const dispatch = useAppDispatch();
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();

  const { query } = useAppSelector(selectQueries);
  const tags = useTags().regularTags;
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
        <Toolbar
          onSelectedApp={(query) => {
            dispatch(setQuery(query));
          }}
        />
        <TagsBar
          query={query}
          tags={tags}
          onRefresh={() => dispatch(actions.refresh())}
          onSetQuery={(q) => dispatch(actions.setQuery(q))}
          onSelectedLabel={(label, query) => {
            dispatch(fetchTagValues({ query, label }));
          }}
        />

        <Box>
          <LoadingOverlay active={isLoadingOrReloading([singleView.type])}>
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
                <ChartTitle
                  className="singleView-timeline-title"
                  titleKey={singleView?.profile?.metadata.units}
                />
              }
              annotations={annotations}
              selectionType="single"
              ContextMenu={contextMenu}
            />
          </LoadingOverlay>
        </Box>
        <Box>
          <LoadingOverlay
            spinnerPosition="baseline"
            active={isLoadingOrReloading([singleView.type])}
          >
            {flamegraphRenderer}
          </LoadingOverlay>
        </Box>
      </div>
    </div>
  );
}

export default ContinuousSingleView;
