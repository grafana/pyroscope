import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import FlameGraphRenderer from './FlameGraph';
import Header from './Header';
import Footer from './Footer';
import TimelineChartWrapper from './TimelineChartWrapper';
import { buildDiffRenderURL } from '../util/updateRequests';
import { fetchNames, fetchComparisonDiffAppData } from '../redux/actions';

function ComparisonDiffApp(props) {
  const { actions, diffRenderURL, diff } = props;
  const prevPropsRef = useRef();

  useEffect(() => {
    if (prevPropsRef.diffRenderURL !== diffRenderURL) {
      actions.fetchComparisonDiffAppData(diffRenderURL);
    }
    return actions.abortTimelineRequest;
  }, [diffRenderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper id="timeline-chart-diff" viewSide="both" />
        <FlameGraphRenderer viewType="diff" flamebearer={diff.flamebearer} />
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  diffRenderURL: buildDiffRenderURL(state.root),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchComparisonDiffAppData,
      fetchNames,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(ComparisonDiffApp);
