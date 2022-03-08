import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import {
  useAppDispatch,
  useAppSelector,
  useOldRootSelector,
} from '@pyroscope/redux/hooks';
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

  const { from, until, query, refreshToken, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  const { singleView } = useAppSelector((state) => state.continuous);

  useEffect(() => {
    if (from && until && query && maxNodes) {
      dispatch(fetchSingleView());
    }
  }, [from, until, query, refreshToken, maxNodes]);

  const getRaw = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return singleView.raw;
      }

      default: {
        return undefined;
      }
    }
  };
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(getRaw());

  const flamegraphRenderer = (() => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return (
          <FlamegraphRenderer
            profile={singleView.profile}
            viewType="single"
            display="both"
            rawFlamegraph={singleView.raw}
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
        );
      }

      default: {
        return 'Loading';
      }
    }
  })();

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper
          data-testid="timeline-single"
          id="timeline-chart-single"
          viewSide="none"
        />
        <Box>{flamegraphRenderer}</Box>
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
