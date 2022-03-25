import React, { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
  fetchTagValues,
} from '@pyroscope/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import Toolbar from '../components/Toolbar';
import Footer from '../components/Footer';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import InstructionText from '../components/InstructionText';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';
import TagsBar from '../components/TagsBar';
import useTags from '../hooks/tags.hook';
import useTimelines, { leftColor, rightColor } from '../hooks/timeline.hook';
import usePopulateLeftRightQuery from '../hooks/populateLeftRightQuery.hook';

function ComparisonDiffApp() {
  const dispatch = useAppDispatch();
  const {
    diffView,
    refreshToken,
    maxNodes,
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
    leftQuery,
    rightQuery,
  } = useAppSelector(selectContinuousState);

  usePopulateLeftRightQuery();
  const { leftTags, rightTags } = useTags({ leftQuery, rightQuery });
  const { leftTimeline, rightTimeline } = useTimelines();

  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    diffView.profile
  );

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

  const exportData = diffView.profile && (
    <ExportData
      flamebearer={diffView.profile}
      exportJSON
      exportPNG
      // disable this until we fix it
      //      exportHTML
      exportFlamegraphDotCom
      exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
    />
  );

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
                  dispatch(fetchTagValues({ query, label }));
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
                  dispatch(fetchTagValues({ query, label }));
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
                  right: { from: rightFrom, to: rightUntil, color: rightColor },
                }}
              />
            </div>
          </div>
          <FlamegraphRenderer
            profile={diffView.profile}
            ExportData={exportData}
          />
        </Box>
      </div>
      <Footer />
    </div>
  );
}

export default ComparisonDiffApp;
