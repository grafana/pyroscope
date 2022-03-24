import React, { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
  selectAppTags,
  fetchTagValues,
  fetchSideTimelines,
  selectTimelineSidesData,
} from '@pyroscope/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import Color from 'color';
import Toolbar from '../components/Toolbar';
import Footer from '../components/Footer';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import InstructionText from '../components/InstructionText';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';
import TagsBar from '../components/TagsBar';

function ComparisonDiffApp() {
  const dispatch = useAppDispatch();
  const {
    diffView,
    from,
    until,
    query,
    refreshToken,
    maxNodes,
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,

    leftQuery,
    rightQuery,
  } = useAppSelector(selectContinuousState);

  const timelines = useAppSelector(selectTimelineSidesData);
  const leftTags = useAppSelector(selectAppTags(leftQuery));
  const rightTags = useAppSelector(selectAppTags(rightQuery));

  // initially populate the queries
  useEffect(() => {
    if (query && !rightQuery) {
      dispatch(actions.setRightQuery(query));
    }
    if (query && !leftQuery) {
      dispatch(actions.setLeftQuery(query));
    }
  }, [query]);

  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    'profile' in diffView ? diffView.profile : undefined
  );

  const profile = (() => {
    switch (diffView.type) {
      case 'loaded':
      case 'reloading': {
        return diffView.profile;
      }
      default:
        // the component is allowed to render without any data
        return undefined;
    }
  })();

  // Only reload timelines when an item that affects a timeline has changed
  useEffect(() => {
    dispatch(fetchSideTimelines(null));
  }, [from, until, refreshToken, maxNodes, leftQuery, rightQuery]);

  useEffect(() => {
    if (rightQuery && leftQuery) {
      dispatch(
        fetchDiffView({
          leftQuery,
          leftFrom,
          leftUntil,

          rightQuery,
          rightFrom,
          rightUntil,
        })
      );
    }
  }, [
    leftFrom,
    leftUntil,
    leftQuery,
    rightFrom,
    rightUntil,
    rightQuery,
    refreshToken,
    maxNodes,
  ]);

  const exportData = profile && (
    <ExportData
      flamebearer={profile}
      exportJSON
      exportPNG
      exportHTML
      //      fetchUrlFunc={() => diffRenderURL}
      exportFlamegraphDotCom
      exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
    />
  );

  // Purple
  const leftColor = Color('rgb(200, 102, 204)');
  // Blue
  const rightColor = Color('rgb(19, 152, 246)');

  const leftTimeline = {
    color: leftColor.rgb().toString(),
    data: timelines.left,
  };

  const rightTimeline = {
    color: rightColor.rgb().toString(),
    data: timelines.right,
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
          id="timeline-chart-diff"
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
        <Box>
          <div className="diff-instructions-wrapper">
            <div className="diff-instructions-wrapper-side">
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
              <InstructionText viewType="diff" viewSide="left" />
              <TimelineChartWrapper
                data-testid="timeline-left"
                key="timeline-chart-left"
                id="timeline-chart-left"
                timelineA={leftTimeline}
                onSelect={(from, until) => {
                  dispatch(actions.setLeft({ from, until }));
                }}
                markings={{
                  left: { from: leftFrom, to: leftUntil, color: leftColor },
                }}
              />
            </div>
            <div className="diff-instructions-wrapper-side">
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
              <InstructionText viewType="diff" viewSide="right" />
              <TimelineChartWrapper
                data-testid="timeline-right"
                key="timeline-chart-right"
                id="timeline-chart-right"
                timelineA={rightTimeline}
                onSelect={(from, until) => {
                  dispatch(actions.setRight({ from, until }));
                }}
                markings={{
                  right: {
                    from: rightFrom,
                    to: rightUntil,
                    color: rightColor,
                  },
                }}
              />
            </div>
          </div>
          <FlamegraphRenderer profile={profile} ExportData={exportData} />
        </Box>
      </div>
      <Footer />
    </div>
  );
}

export default ComparisonDiffApp;
