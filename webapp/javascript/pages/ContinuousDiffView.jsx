import React, { useEffect, useRef } from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import Header from '../components/Header';
import Footer from '../components/Footer';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import { buildDiffRenderURL } from '../util/updateRequests';
import { fetchNames, fetchComparisonDiffAppData } from '../redux/actions';
import InstructionText from '../components/InstructionText';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';

function ComparisonDiffApp(props) {
  const { actions, diffRenderURL, diff } = props;
  const prevPropsRef = useRef();
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(diff.raw);

  useEffect(() => {
    if (prevPropsRef.diffRenderURL !== diffRenderURL) {
      actions.fetchComparisonDiffAppData(diffRenderURL);
    }
    return actions.abortTimelineRequest;
  }, [diffRenderURL]);

  const exportData = (
    <ExportData
      flamebearer={diff.raw}
      exportJSON
      exportPNG
      exportHTML
      fetchUrlFunc={() => diffRenderURL}
      exportFlamegraphDotCom
      exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
    />
  );

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Header />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-diff"
          viewSide="both"
        />
        <Box>
          <FlamegraphRenderer
            display="both"
            viewType="diff"
            flamebearer={diff.flamebearer}
            rawFlamegraph={diff}
            ExportData={exportData}
          >
            <div className="diff-instructions-wrapper">
              <div className="diff-instructions-wrapper-side">
                <InstructionText viewType="diff" viewSide="left" />
                <TimelineChartWrapper
                  data-testid="timeline-left"
                  key="timeline-chart-left"
                  id="timeline-chart-left"
                  viewSide="left"
                />
              </div>
              <div className="diff-instructions-wrapper-side">
                <InstructionText viewType="diff" viewSide="right" />
                <TimelineChartWrapper
                  data-testid="timeline-right"
                  key="timeline-chart-right"
                  id="timeline-chart-right"
                  viewSide="right"
                />
              </div>
            </div>
          </FlamegraphRenderer>
        </Box>
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
