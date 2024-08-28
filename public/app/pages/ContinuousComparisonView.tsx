import React, { useEffect } from 'react';
import 'react-dom';

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
import TagsBar from '@pyroscope/components/TagsBar';
import ChartTitle from '@pyroscope/components/ChartTitle';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
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
import { Panel } from '@pyroscope/components/Panel';
import { PageContentWrapper } from '@pyroscope/pages/PageContentWrapper';
import { Profile } from '@pyroscope/legacy/models/profile';
import { SharedQuery } from '@pyroscope/legacy/flamegraph/FlameGraph/FlameGraphRenderer';
import { FlameGraphWrapper } from '@pyroscope/components/FlameGraphWrapper';

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
        <Panel
          isLoading={isLoading}
          title={
            <ChartTitle
              titleKey={
                isSidesHasSameUnits
                  ? (leftSide.metadata.name as any)
                  : undefined
              }
            />
          }
        >
          <TimelineChartWrapper
            data-testid="timeline-main"
            id="timeline-chart-double"
            format="lines"
            height="125px"
            annotations={annotations}
            timelineA={leftTimeline}
            timelineB={rightTimeline}
            onSelect={handleSelectMain}
            syncCrosshairsWith={['timeline-chart-left', 'timeline-chart-right']}
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
        </Panel>
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Panel
            dataTestId="baseline-panel"
            isLoading={isLoading}
            className={styles.comparisonPane}
            title={<ChartTitle titleKey="baseline" color={leftColor} />}
            headerActions={
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
            }
          >
            <TagsBar
              query={leftQuery}
              tags={leftTags}
              onRefresh={() => dispatch(actions.refresh())}
              onSetQuery={(q) => dispatch(actions.setLeftQuery(q))}
              onSelectedLabel={(label, query) => {
                dispatch(fetchTagValues({ query, label }));
              }}
            />
            <FlameGraphAndTimeline
              position={'left'}
              profile={leftSide}
              from={leftFrom}
              to={leftUntil}
              sharedQuery={{ ...sharedQuery, id: 'left' }}
              handleSelect={handleSelectLeft}
              timezone={timezone}
              timeline={leftTimeline}
            />
          </Panel>

          <Panel
            dataTestId="comparison-panel"
            isLoading={isLoading}
            className={styles.comparisonPane}
            title={<ChartTitle titleKey="comparison" color={rightColor} />}
          >
            <TagsBar
              query={rightQuery}
              tags={rightTags}
              onRefresh={() => dispatch(actions.refresh())}
              onSetQuery={(q) => dispatch(actions.setRightQuery(q))}
              onSelectedLabel={(label, query) => {
                dispatch(fetchTagValues({ query, label }));
              }}
            />
            <FlameGraphAndTimeline
              position={'right'}
              profile={rightSide}
              from={rightFrom}
              to={rightUntil}
              sharedQuery={{ ...sharedQuery, id: 'right' }}
              handleSelect={handleSelectRight}
              timezone={timezone}
              timeline={rightTimeline}
            />
          </Panel>
        </div>
      </PageContentWrapper>
    </div>
  );
}

function FlameGraphAndTimeline(props: {
  profile: Profile | undefined;
  sharedQuery: SharedQuery;
  position: 'left' | 'right';
  from: string;
  to: string;
  handleSelect: (from: string, until: string) => void;
  timezone: 'browser' | 'utc';
  timeline: TimelineData;
}) {
  const {
    profile,
    to,
    from,
    handleSelect,
    timezone,
    sharedQuery,
    timeline,
    position,
  } = props;

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

  return (
    <FlameGraphWrapper
      profile={profile}
      vertical={true}
      timelineEl={timelineEl}
      sharedQuery={sharedQuery}
      dataTestId={
        position === 'right'
          ? 'flamegraph-renderer-right'
          : 'flamegraph-renderer-left'
      }
    />
  );
}

export default ComparisonApp;
