import React, { useEffect, useRef, useState } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import FlameGraphRenderer from './FlameGraph';
import TimelineChartWrapper from './TimelineChartWrapper';
import Header from './Header';
import Footer from './Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchComparisonAppData,
  fetchTimeline,
} from '../redux/actions';
import InstructionText from './FlameGraph/InstructionText';
import styles from './ComparisonApp.module.css';

// See docs here: https://github.com/flot/flot/blob/master/API.md

function ComparisonApp(props) {
  const { actions, renderURL, leftRenderURL, rightRenderURL, comparison } =
    props;
  const { rawLeft, rawRight } = comparison;

  const [linkedSearch, setLinkedSearch] = useState({
    isSearchLinked: false,
    linkedSearchQuery: '',
    resetLinkedSearchSide: '',
  });

  const setSearchQuery = (query) => {
    setLinkedSearch((x) => {
      return {
        ...x,
        linkedSearchQuery: query,
      };
    });
  };
  const toggleLinkedSearch = (side) => {
    const { isSearchLinked } = linkedSearch;
    if (isSearchLinked) {
      if (side === 'left') {
        setLinkedSearch((x) => {
          return {
            ...x,
            resetLinkedSearchSide: 'right',
          };
        });
      }
      if (side === 'right') {
        setLinkedSearch((x) => {
          return {
            ...x,
            resetLinkedSearchSide: 'left',
          };
        });
      }
      if (side === 'both') {
        setLinkedSearch((x) => {
          return { ...x, isSearchLinked: false, resetLinkedSearchSide: '' };
        });
      }
    } else {
      setLinkedSearch((x) => {
        return { ...x, isSearchLinked: true, resetLinkedSearchSide: '' };
      });
    }
  };

  useEffect(() => {
    actions.fetchComparisonAppData(leftRenderURL, 'left');
    return actions.abortTimelineRequest;
  }, [leftRenderURL]);

  useEffect(() => {
    actions.fetchComparisonAppData(rightRenderURL, 'right');
    return actions.abortTimelineRequest;
  }, [rightRenderURL]);

  useEffect(() => {
    actions.fetchTimeline(renderURL);

    return actions.abortTimelineRequest;
  }, [renderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-double"
          viewSide="both"
        />
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <FlameGraphRenderer
              viewType="double"
              viewSide="left"
              flamebearer={comparison.left.flamebearer}
              data-testid="flamegraph-renderer-left"
              display="both"
              rawFlamegraph={rawLeft}
              isSearchLinked={linkedSearch.isSearchLinked}
              setSearchQuery={setSearchQuery}
              linkedSearchQuery={linkedSearch.linkedSearchQuery}
              toggleLinkedSearch={toggleLinkedSearch}
              resetLinkedSearchSide={linkedSearch.resetLinkedSearchSide}
            >
              <InstructionText viewType="double" viewSide="left" />
              <TimelineChartWrapper
                key="timeline-chart-left"
                id="timeline-chart-left"
                data-testid="timeline-left"
                viewSide="left"
              />
            </FlameGraphRenderer>
          </Box>

          <Box className={styles.comparisonPane}>
            <FlameGraphRenderer
              viewType="double"
              viewSide="right"
              flamebearer={comparison.right.flamebearer}
              data-testid="flamegraph-renderer-right"
              display="both"
              rawFlamegraph={rawRight}
              isSearchLinked={linkedSearch.isSearchLinked}
              setSearchQuery={setSearchQuery}
              linkedSearchQuery={linkedSearch.linkedSearchQuery}
              toggleLinkedSearch={toggleLinkedSearch}
              resetLinkedSearchSide={linkedSearch.resetLinkedSearchSide}
            >
              <InstructionText viewType="double" viewSide="right" />
              <TimelineChartWrapper
                key="timeline-chart-right"
                id="timeline-chart-right"
                data-testid="timeline-right"
                viewSide="right"
              />
            </FlameGraphRenderer>
          </Box>
        </div>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  renderURL: buildRenderURL(state.root),
  leftRenderURL: buildRenderURL(
    state.root,
    state.root.leftFrom,
    state.root.leftUntil
  ),
  rightRenderURL: buildRenderURL(
    state.root,
    state.root.rightFrom,
    state.root.rightUntil
  ),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchComparisonAppData,
      fetchNames,
      fetchTimeline,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(ComparisonApp);
