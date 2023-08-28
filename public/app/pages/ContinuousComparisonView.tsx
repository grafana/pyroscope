import React, { useEffect } from 'react';
import 'react-dom';

import Box from '@pyroscope/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  selectContinuousState,
  actions,
  selectComparisonState,
  fetchComparisonSide,
  fetchTagValues,
  selectQueries,
  selectTimelineSides,
  selectAnnotationsOrDefault,
} from '@pyroscope/redux/reducers/continuous';
import SideTimelineComparator from '@pyroscope/components/SideTimelineComparator';
import TimelineChartWrapper from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import SyncTimelines from '@pyroscope/components/TimelineChart/SyncTimelines';
import Toolbar from '@pyroscope/components/Toolbar';
import ExportData from '@pyroscope/components/ExportData';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import TagsBar from '@pyroscope/components/TagsBar';
import ChartTitle from '@pyroscope/components/ChartTitle';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import { isExportToFlamegraphDotComEnabled } from '@pyroscope/util/features';
import { LoadingOverlay } from '@pyroscope/ui/LoadingOverlay';
import PageTitle from '@pyroscope/components/PageTitle';
import { Query } from '@pyroscope/models/query';
import { isLoadingOrReloading } from '@pyroscope/pages/loading';
import styles from './ContinuousComparison.module.css';
import useTags from '../hooks/tags.hook';
import useTimelines, {
  leftColor,
  rightColor,
  selectionColor,
} from '../hooks/timeline.hook';
import usePopulateLeftRightQuery from '../hooks/populateLeftRightQuery.hook';
import useFlamegraphSharedQuery from '../hooks/flamegraphSharedQuery.hook';
import { formatTitle } from './formatTitle';
import { PageContentWrapper } from './layout';

function ComparisonApp() {
  const dispatch = useAppDispatch();
  const {
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
    refreshToken,
    from,
    until,
  } = useAppSelector(selectContinuousState);
  const { leftQuery, rightQuery } = useAppSelector(selectQueries);
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();
  usePopulateLeftRightQuery();
  const {
    left: comparisonLeft,
    right: comparisonRight,
    comparisonMode,
  } = useAppSelector(selectComparisonState);
  const { leftTags, rightTags } = useTags();
  const { leftTimeline, rightTimeline } = useTimelines();
  const sharedQuery = useFlamegraphSharedQuery();
  const annotations = useAppSelector(
    selectAnnotationsOrDefault('comparisonView')
  );

  const timelines = useAppSelector(selectTimelineSides);
  const isLoading = isLoadingOrReloading([
    comparisonLeft.type,
    comparisonRight.type,
    timelines.left.type,
    timelines.right.type,
  ]);

  useEffect(() => {
    if (leftQuery) {
      const fetchLeftQueryData = dispatch(
        fetchComparisonSide({ side: 'left', query: leftQuery })
      );
      return fetchLeftQueryData.abort;
    }
    return undefined;
  }, [dispatch, leftFrom, leftUntil, leftQuery, refreshToken]);

  useEffect(() => {
    if (rightQuery) {
      const fetchRightQueryData = dispatch(
        fetchComparisonSide({ side: 'right', query: rightQuery })
      );

      return fetchRightQueryData.abort;
    }
    return undefined;
  }, [dispatch, rightFrom, rightUntil, rightQuery, refreshToken]);

  const leftSide = comparisonLeft.profile;
  const rightSide = comparisonRight.profile;
  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(leftSide);
  const exportToFlamegraphDotComRightFn =
    useExportToFlamegraphDotCom(rightSide);
  const timezone = offset === 0 ? 'utc' : 'browser';
  const isSidesHasSameUnits =
    leftSide &&
    rightSide &&
    leftSide.metadata.units === rightSide.metadata.units;

  const handleCompare = ({
    from,
    until,
    leftFrom,
    leftTo,
    rightFrom,
    rightTo,
  }: {
    from: string;
    until: string;
    leftFrom: string;
    leftTo: string;
    rightFrom: string;
    rightTo: string;
  }) => {
    dispatch(
      actions.setFromAndUntil({
        from,
        until,
      })
    );
    dispatch(actions.setRight({ from: rightFrom, until: rightTo }));
    dispatch(actions.setLeft({ from: leftFrom, until: leftTo }));
  };

  const setComparisonMode = (mode: {
    active: boolean;
    period: {
      label: string;
      ms: number;
    };
  }) => {
    dispatch(actions.setComparisonMode(mode));
  };

  const handleSelectMain = (from: string, until: string) => {
    setComparisonMode({
      ...comparisonMode,
      active: false,
    });
    dispatch(actions.setFromAndUntil({ from, until }));
  };

  const handleSelectLeft = (from: string, until: string) => {
    setComparisonMode({
      ...comparisonMode,
      active: false,
    });
    dispatch(actions.setLeft({ from, until }));
  };

  const handleSelectRight = (from: string, until: string) => {
    setComparisonMode({
      ...comparisonMode,
      active: false,
    });
    dispatch(actions.setRight({ from, until }));
  };

  const handleSelectedApp = (query: Query) => {
    setComparisonMode({
      ...comparisonMode,
      active: false,
    });
    dispatch(actions.setQuery(query));
  };

  return (
    <div>
      <PageTitle title={formatTitle('Comparison', leftQuery, rightQuery)} />
      <PageContentWrapper>
        <Toolbar onSelectedApp={handleSelectedApp} />
        <Box>
          <LoadingOverlay active={isLoading}>
            <TimelineChartWrapper
              data-testid="timeline-main"
              id="timeline-chart-double"
              format="lines"
              height="125px"
              annotations={annotations}
              timelineA={leftTimeline}
              timelineB={rightTimeline}
              onSelect={handleSelectMain}
              syncCrosshairsWith={[
                'timeline-chart-left',
                'timeline-chart-right',
              ]}
              selection={{
                left: {
                  from: leftFrom,
                  to: leftUntil,
                  color: leftColor,
                  overlayColor: leftColor.alpha(0.3),
                },
                right: {
                  from: rightFrom,
                  to: rightUntil,
                  color: rightColor,
                  overlayColor: rightColor.alpha(0.3),
                },
              }}
              timezone={timezone}
              title={
                <ChartTitle
                  titleKey={
                    isSidesHasSameUnits ? leftSide.metadata.units : undefined
                  }
                />
              }
              selectionType="double"
            />
            <SyncTimelines
              isDataLoading={isLoading}
              comparisonModeActive={comparisonMode.active}
              timeline={leftTimeline}
              leftSelection={{ from: leftFrom, to: leftUntil }}
              rightSelection={{ from: rightFrom, to: rightUntil }}
              onSync={(from, until) => {
                dispatch(actions.setFromAndUntil({ from, until }));
              }}
            />
          </LoadingOverlay>
        </Box>
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <LoadingOverlay active={isLoading} spinnerPosition="baseline">
              <div className={styles.timelineTitleWrapper}>
                <ChartTitle titleKey="baseline" color={leftColor} />
                <SideTimelineComparator
                  setComparisonMode={setComparisonMode}
                  comparisonMode={comparisonMode}
                  onCompare={handleCompare}
                  selection={{
                    from,
                    until,
                    left: {
                      from: leftFrom,
                      to: leftUntil,
                      color: leftColor,
                      overlayColor: leftColor.alpha(0.3),
                    },
                    right: {
                      from: rightFrom,
                      to: rightUntil,
                      color: rightColor,
                      overlayColor: rightColor.alpha(0.3),
                    },
                  }}
                />
              </div>

              <TagsBar
                query={leftQuery}
                tags={leftTags}
                onRefresh={() => dispatch(actions.refresh())}
                onSetQuery={(q) => dispatch(actions.setLeftQuery(q))}
                onSelectedLabel={(label, query) => {
                  dispatch(fetchTagValues({ query, label }));
                }}
              />
              <FlamegraphRenderer
                showCredit={false}
                panesOrientation="vertical"
                profile={leftSide}
                data-testid="flamegraph-renderer-left"
                colorMode={colorMode}
                sharedQuery={{ ...sharedQuery, id: 'left' }}
                ExportData={
                  // Don't export PNG since the exportPng code is broken
                  leftSide && (
                    <ExportData
                      flamebearer={leftSide}
                      exportJSON
                      exportHTML
                      exportPprof
                      exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
                      exportFlamegraphDotComFn={exportToFlamegraphDotComLeftFn}
                    />
                  )
                }
              >
                <TimelineChartWrapper
                  key="timeline-chart-left"
                  id="timeline-chart-left"
                  data-testid="timeline-left"
                  selectionWithHandler
                  syncCrosshairsWith={[
                    'timeline-chart-double',
                    'timeline-chart-right',
                  ]}
                  timelineA={leftTimeline}
                  selection={{
                    left: {
                      from: leftFrom,
                      to: leftUntil,
                      color: selectionColor,
                      overlayColor: selectionColor.alpha(0.3),
                    },
                  }}
                  selectionType="single"
                  onSelect={handleSelectLeft}
                  timezone={timezone}
                />
              </FlamegraphRenderer>
            </LoadingOverlay>
          </Box>

          <Box className={styles.comparisonPane}>
            <LoadingOverlay spinnerPosition="baseline" active={isLoading}>
              <div className={styles.timelineTitleWrapper}>
                <ChartTitle titleKey="comparison" color={rightColor} />
              </div>
              <TagsBar
                query={rightQuery}
                tags={rightTags}
                onRefresh={() => dispatch(actions.refresh())}
                onSetQuery={(q) => dispatch(actions.setRightQuery(q))}
                onSelectedLabel={(label, query) => {
                  dispatch(fetchTagValues({ query, label }));
                }}
              />
              <FlamegraphRenderer
                showCredit={false}
                profile={rightSide}
                data-testid="flamegraph-renderer-right"
                panesOrientation="vertical"
                colorMode={colorMode}
                sharedQuery={{ ...sharedQuery, id: 'right' }}
                ExportData={
                  // Don't export PNG since the exportPng code is broken
                  rightSide && (
                    <ExportData
                      flamebearer={rightSide}
                      exportJSON
                      exportHTML
                      exportPprof
                      exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
                      exportFlamegraphDotComFn={exportToFlamegraphDotComRightFn}
                    />
                  )
                }
              >
                <TimelineChartWrapper
                  key="timeline-chart-right"
                  id="timeline-chart-right"
                  data-testid="timeline-right"
                  timelineA={rightTimeline}
                  syncCrosshairsWith={[
                    'timeline-chart-double',
                    'timeline-chart-left',
                  ]}
                  selectionWithHandler
                  selection={{
                    right: {
                      from: rightFrom,
                      to: rightUntil,
                      color: selectionColor,
                      overlayColor: selectionColor.alpha(0.3),
                    },
                  }}
                  selectionType="single"
                  onSelect={handleSelectRight}
                  timezone={timezone}
                />
              </FlamegraphRenderer>
            </LoadingOverlay>
          </Box>
        </div>
      </PageContentWrapper>
    </div>
  );
}

export default ComparisonApp;
