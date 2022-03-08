import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { useAppDispatch, useOldRootSelector } from '@pyroscope/redux/hooks';
import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { fetchSingleView } from '@pyroscope/redux/reducers/continuous';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import Header from '../components/Header';
import Footer from '../components/Footer';
import { buildRenderURL } from '../util/updateRequests';
import {
  fetchNames,
  fetchPyroscopeAppData,
  abortTimelineRequest,
} from '../redux/actions';
import ExportData from '../components/ExportData';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';

function PyroscopeApp(props) {
  const { actions, renderURL, single, raw } = props;
  const prevPropsRef = useRef();
  const dispatch = useAppDispatch();
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(raw);

  const { from, until, query, refreshToken, maxNodes } = useOldRootSelector(
    (state) => state
  );

  useEffect(() => {
    console.log('dispatching new request');
    dispatch(fetchSingleView());
    //    dispatch(
    //      fetchSingleView({
    //        from,
    //        until,
    //        query,
    //        refreshToken,
    //        maxNodes,
    //      })
    //    );
  }, [from, until, query, refreshToken, maxNodes]);

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
          <FlamegraphRenderer
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
                exportFlamegraphDotCom
                exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
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
