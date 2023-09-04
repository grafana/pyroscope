import React, { useEffect } from 'react';
import 'react-dom';

import { createTheme } from '@grafana/data';
import { FlameGraph } from '@grafana/flamegraph';
import { Button, Tooltip } from '@grafana/ui';

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
import TimelineChartWrapper, {
  TimelineData,
} from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import SyncTimelines from '@pyroscope/components/TimelineChart/SyncTimelines';
import Toolbar from '@pyroscope/components/Toolbar';
import ExportData from '@pyroscope/components/ExportData';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import TagsBar from '@pyroscope/components/TagsBar';
import ChartTitle from '@pyroscope/components/ChartTitle';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import {
  isExportToFlamegraphDotComEnabled,
  isGrafanaFlamegraphEnabled,
} from '@pyroscope/util/features';
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
import { Profile } from '@pyroscope/legacy/models/profile';
import { SharedQuery } from '@pyroscope/legacy/flamegraph/FlameGraph/FlameGraphRenderer';
import { diffFlamebearerToDataFrameDTO } from '@pyroscope/util/flamebearer';

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

              <FlamegraphWrapper
                position={'left'}
                profile={leftSide}
                from={leftFrom}
                to={leftUntil}
                sharedQuery={{ ...sharedQuery, id: 'left' }}
                handleSelect={handleSelectLeft}
                timezone={timezone}
                timeline={leftTimeline}
                colorMode={colorMode}
              />
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
              <FlamegraphWrapper
                position={'right'}
                profile={rightSide}
                from={rightFrom}
                to={rightUntil}
                sharedQuery={{ ...sharedQuery, id: 'right' }}
                handleSelect={handleSelectRight}
                timezone={timezone}
                timeline={rightTimeline}
                colorMode={colorMode}
              />
            </LoadingOverlay>
          </Box>
        </div>
      </PageContentWrapper>
    </div>
  );
}

function FlamegraphWrapper(props: {
  profile: Profile | undefined;
  sharedQuery: SharedQuery;
  position: 'left' | 'right';
  from: string;
  to: string;
  handleSelect: (from: string, until: string) => void;
  timezone: 'browser' | 'utc';
  timeline: TimelineData;
  colorMode: 'light' | 'dark';
}) {
  const {
    profile,
    to,
    from,
    handleSelect,
    timezone,
    sharedQuery,
    timeline,
    colorMode,
    position,
  } = props;
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(profile);

  const id =
    position === 'right' ? 'timeline-chart-right' : 'timeline-chart-left';
  const testid = position === 'right' ? 'timeline-right' : 'timeline-left';

  const timelineEl = (
    <TimelineChartWrapper
      key={id}
      id={id}
      data-testid={testid}
      timelineA={timeline}
      syncCrosshairsWith={['timeline-chart-double', id]}
      selectionWithHandler
      selection={{
        right: {
          from,
          to,
          color: selectionColor,
          overlayColor: selectionColor.alpha(0.3),
        },
      }}
      selectionType="single"
      onSelect={handleSelect}
      timezone={timezone}
    />
  );

  if (isGrafanaFlamegraphEnabled) {
    const dataFrame = profile
      ? diffFlamebearerToDataFrameDTO(
          profile.flamebearer.levels,
          profile.flamebearer.names
        )
      : undefined;
    return (
      <>
        {timelineEl}
        <FlameGraph
          getTheme={() => createTheme({ colors: { mode: 'dark' } })}
          data={dataFrame}
          extraHeaderElements={
            profile && (
              <ExportData
                flamebearer={profile}
                exportJSON
                exportPprof
                exportHTML
                exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
                exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
                buttonEl={({ onClick }) => {
                  return (
                    <Tooltip content={'Export Data'}>
                      <Button
                        // Ugly hack to go around globally defined line height messing up sizing of the button.
                        // Not sure why it happens even if everything is display: Block. To override it would
                        // need changes in Flamegraph which would be weird so this seems relatively sensible.
                        style={{ marginTop: -7 }}
                        icon={'download-alt'}
                        size={'sm'}
                        variant={'secondary'}
                        fill={'outline'}
                        onClick={onClick}
                      />
                    </Tooltip>
                  );
                }}
              />
            )
          }
        />
      </>
    );
  } else {
    return (
      <>
        {timelineEl}
        <FlamegraphRenderer
          showCredit={false}
          profile={profile}
          data-testid={
            position === 'right'
              ? 'flamegraph-renderer-right'
              : 'flamegraph-renderer-left'
          }
          panesOrientation="vertical"
          colorMode={colorMode}
          sharedQuery={sharedQuery}
          ExportData={
            // Don't export PNG since the exportPng code is broken
            profile && (
              <ExportData
                flamebearer={profile}
                exportJSON
                exportHTML
                exportPprof
                exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
                exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
              />
            )
          }
        />
      </>
    );
  }
}

export default ComparisonApp;
