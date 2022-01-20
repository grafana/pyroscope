import React, { useEffect, useRef } from 'react';
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
  fetchPyroscopeAppData,
  abortTimelineRequest,
} from '../redux/actions';
import ExportData from './ExportData';

function PyroscopeApp(props) {
  const { actions, renderURL, single, raw } = props;
  const prevPropsRef = useRef();

  console.log(JSON.stringify(raw));

  useEffect(() => {
    if (prevPropsRef.renderURL !== renderURL) {
      actions.fetchPyroscopeAppData(renderURL);
    }

    return actions.abortTimelineRequest;
  }, [renderURL]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper
          data-testid="timeline-single"
          id="timeline-chart-single"
          viewSide="none"
        />
        <Box>
          <FlameGraphRenderer
            flamebearer={single?.flamebearer}
            viewType="single"
            display="both"
            rawFlamegraph={raw}
            ExportData={
              // Don't export PNG since the exportPng code is broken
              <ExportData
                flamebearer={raw}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
              />
            }
          />
        </Box>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  renderURL: buildRenderURL(state.root),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchPyroscopeAppData,
      fetchNames,
      abortTimelineRequest,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(PyroscopeApp);
