import React, { useEffect, useRef } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
  selectAppTags,
  fetchTagValues,
  selectComparisonState,
  fetchComparisonSide,
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

  const getRaw = () => {
    switch (diffView.type) {
      case 'loaded':
      case 'reloading': {
        return diffView.profile;
      }

      default: {
        return undefined;
      }
    }
  };
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(getRaw());

  useEffect(() => {
    dispatch(fetchDiffView(null));
  }, [
    from,
    until,
    query,
    refreshToken,
    maxNodes,
    rightFrom,
    leftFrom,
    leftUntil,
    rightUntil,
  ]);

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

  // Every time one of the queries changes, we need to actually refresh BOTH
  // otherwise one of the timelines will be outdated
  useEffect(() => {
    if (leftQuery) {
      dispatch(fetchComparisonSide({ side: 'left', query: leftQuery }));
    }

    if (rightQuery) {
      dispatch(fetchComparisonSide({ side: 'right', query: rightQuery }));
    }

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

  const getTimeline = () => {
    switch (diffView.type) {
      case 'loaded':
      case 'reloading': {
        return {
          data: diffView.timeline,
        };
      }

      default: {
        return {
          data: undefined,
        };
      }
    }
  };

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
              <InstructionText viewType="diff" viewSide="left" />

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
              <InstructionText viewType="diff" viewSide="right" />

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
          <FlamegraphRenderer
            display="both"
            viewType="diff"
            profile={profile}
            ExportData={exportData}
          />
        </Box>
      </div>
      <Footer />
    </div>
  );
}

export default ComparisonDiffApp;
