import React, { useEffect, useRef } from 'react';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
} from '@pyroscope/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import Toolbar from '../components/Toolbar';
import Footer from '../components/Footer';
import TimelineChartWrapper from '../components/TimelineChartWrapper';
import InstructionText from '../components/InstructionText';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';

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
  } = useAppSelector(selectContinuousState);

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

  const getTimelineData = () => {
    switch (diffView.type) {
      case 'loaded':
      case 'reloading': {
        return diffView.timeline;
      }

      default:
        return undefined;
    }
  };

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-diff"
          viewSide="both"
          timeline={getTimelineData()}
          leftFrom={leftFrom}
          leftUntil={leftUntil}
          rightFrom={rightFrom}
          rightUntil={rightUntil}
          onSelect={(from, until) => {
            dispatch(actions.setFromAndUntil({ from, until }));
          }}
        />
        <Box>
          <FlamegraphRenderer
            display="both"
            viewType="diff"
            profile={profile}
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
                  timeline={getTimelineData()}
                  leftFrom={leftFrom}
                  leftUntil={leftUntil}
                  rightFrom={rightFrom}
                  rightUntil={rightUntil}
                  onSelect={(from, until) => {
                    dispatch(actions.setLeft({ from, until }));
                  }}
                />
              </div>
              <div className="diff-instructions-wrapper-side">
                <InstructionText viewType="diff" viewSide="right" />

                <TimelineChartWrapper
                  data-testid="timeline-right"
                  key="timeline-chart-right"
                  id="timeline-chart-right"
                  viewSide="right"
                  leftFrom={leftFrom}
                  leftUntil={leftUntil}
                  rightFrom={rightFrom}
                  rightUntil={rightUntil}
                  timeline={getTimelineData()}
                  onSelect={(from, until) => {
                    dispatch(actions.setRight({ from, until }));
                  }}
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

export default ComparisonDiffApp;
