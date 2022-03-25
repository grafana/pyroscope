import React, { useEffect } from 'react';
import 'react-dom';

import Box from '@webapp/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  selectContinuousState,
  selectAppTags,
  actions,
  selectComparisonState,
  fetchComparisonSide,
  fetchTags,
  fetchTagValues,
} from '@webapp/redux/reducers/continuous';
import Color from 'color';
import TimelineChartWrapper from '@webapp/components/TimelineChartWrapper';
import Toolbar from '@webapp/components/Toolbar';
import Footer from '@webapp/components/Footer';
import InstructionText from '@webapp/components/InstructionText';
import ExportData from '@webapp/components/ExportData';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import TagsBar from '@webapp/components/TagsBar';
import styles from './ContinuousComparison.module.css';

function ComparisonApp() {
  const dispatch = useAppDispatch();
  const {
    from,
    until,
    query,
    leftQuery,
    rightQuery,
    refreshToken,
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
  } = useAppSelector(selectContinuousState);

  const leftTags = useAppSelector(selectAppTags(leftQuery));
  const rightTags = useAppSelector(selectAppTags(rightQuery));

  const comparisonView = useAppSelector(selectComparisonState);

  // initially populate the queries
  useEffect(() => {
    if (query && !rightQuery) {
      dispatch(actions.setRightQuery(query));
    }
    if (query && !leftQuery) {
      dispatch(actions.setLeftQuery(query));
    }
  }, [query]);

  useEffect(() => {
    // TODO if the query is the same the request will be made twice
    if (leftQuery) {
      dispatch(fetchTags(leftQuery));
    }
    if (rightQuery) {
      dispatch(fetchTags(rightQuery));
    }
  }, [leftQuery, rightQuery]);

  // Every time one of the queries changes, we need to actually refresh BOTH
  // otherwise one of the timelines will be outdated
  useEffect(() => {
    if (leftQuery) {
      dispatch(fetchComparisonSide({ side: 'left', query: leftQuery }));
    }

    if (rightQuery) {
      dispatch(fetchComparisonSide({ side: 'right', query: rightQuery }));
    }
  }, [
    leftFrom,
    leftUntil,
    leftQuery,
    rightFrom,
    rightUntil,
    rightQuery,
    from,
    until,
    refreshToken,
    from,
    until,
  ]);

  const getSide = (side: 'left' | 'right') => {
    const s = comparisonView[side];

    switch (s.type) {
      case 'loaded':
      case 'reloading': {
        return s;
      }

      default:
        return { timeline: undefined, profile: undefined };
    }
  };

  const leftSide = getSide('left');
  const rightSide = getSide('right');

  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(
    leftSide.profile
  );
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(
    leftSide.profile
  );

  // Purple
  const leftColor = Color('rgb(200, 102, 204)');
  // Blue
  const rightColor = Color('rgb(19, 152, 246)');

  const leftTimeline = {
    color: leftColor.rgb().toString(),
    data: leftSide.timeline,
  };

  const rightTimeline = {
    color: rightColor.rgb().toString(),
    data: rightSide.timeline,
  };

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar
          hideTagsBar
          onSelectedName={(query) => {
            dispatch(actions.setRightQuery(query));
            dispatch(actions.setLeftQuery(query));
            dispatch(actions.setQuery(query));
          }}
        />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-double"
          format="lines"
          timelineA={leftTimeline}
          timelineB={rightTimeline}
          onSelect={(from, until) => {
            dispatch(actions.setFromAndUntil({ from, until }));
          }}
          markings={{
            left: { from: leftFrom, to: leftUntil, color: leftColor },
            right: { from: rightFrom, to: rightUntil, color: rightColor },
          }}
        />
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <TagsBar
              query={leftQuery || ''}
              tags={leftTags}
              onSetQuery={(q) => {
                dispatch(actions.setLeftQuery(q));
              }}
              onSelectedLabel={(label, query) => {
                dispatch(
                  fetchTagValues({
                    query,
                    label,
                  })
                );
              }}
            />
            <FlamegraphRenderer
              panesOrientation="vertical"
              profile={leftSide.profile}
              data-testid="flamegraph-renderer-left"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                leftSide.profile && (
                  <ExportData
                    flamebearer={leftSide.profile}
                    exportJSON
                    exportHTML
                    exportPprof
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComLeftFn}
                  />
                )
              }
            >
              <InstructionText viewType="double" viewSide="left" />
              <TimelineChartWrapper
                key="timeline-chart-left"
                id="timeline-chart-left"
                data-testid="timeline-left"
                timelineA={leftTimeline}
                markings={{
                  left: { from: leftFrom, to: leftUntil, color: leftColor },
                }}
                onSelect={(from, until) => {
                  dispatch(actions.setLeft({ from, until }));
                }}
              />
            </FlamegraphRenderer>
          </Box>

          <Box className={styles.comparisonPane}>
            <TagsBar
              query={rightQuery || ''}
              tags={rightTags}
              onSetQuery={(q) => {
                dispatch(actions.setRightQuery(q));
              }}
              onSelectedLabel={(label, query) => {
                dispatch(
                  fetchTagValues({
                    query,
                    label,
                  })
                );
              }}
            />
            <FlamegraphRenderer
              profile={rightSide.profile}
              data-testid="flamegraph-renderer-right"
              panesOrientation="vertical"
              ExportData={
                // Don't export PNG since the exportPng code is broken
                rightSide.profile && (
                  <ExportData
                    flamebearer={rightSide.profile}
                    exportJSON
                    exportHTML
                    exportPprof
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComRightFn}
                  />
                )
              }
            >
              <InstructionText viewType="double" viewSide="right" />
              <TimelineChartWrapper
                key="timeline-chart-right"
                id="timeline-chart-right"
                data-testid="timeline-right"
                timelineA={rightTimeline}
                markings={{
                  right: { from: rightFrom, to: rightUntil, color: rightColor },
                }}
                onSelect={(from, until) => {
                  dispatch(actions.setRight({ from, until }));
                }}
              />
            </FlamegraphRenderer>
          </Box>
        </div>
      </div>
      <Footer />
    </div>
  );
}

export default ComparisonApp;
