import React, { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
} from '@webapp/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import Color from 'color';
import Toolbar from '@webapp/components/Toolbar';
import Footer from '@webapp/components/Footer';
import TimelineChartWrapper from '@webapp/components/TimelineChartWrapper';
import InstructionText from '@webapp/components/InstructionText';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import ExportData from '@webapp/components/ExportData';

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

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Toolbar />
        <TimelineChartWrapper
          data-testid="timeline-main"
          id="timeline-chart-diff"
          timelineA={getTimeline()}
          onSelect={(from, until) => {
            dispatch(actions.setFromAndUntil({ from, until }));
          }}
          markings={{
            left: { from: leftFrom, to: leftUntil, color: leftColor },
            right: { from: rightFrom, to: rightUntil, color: rightColor },
          }}
        />
        <Box>
          <FlamegraphRenderer profile={profile} ExportData={exportData}>
            <div className="diff-instructions-wrapper">
              <div className="diff-instructions-wrapper-side">
                <InstructionText viewType="diff" viewSide="left" />

                <TimelineChartWrapper
                  data-testid="timeline-left"
                  key="timeline-chart-left"
                  id="timeline-chart-left"
                  timelineA={getTimeline()}
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

                <TimelineChartWrapper
                  data-testid="timeline-right"
                  key="timeline-chart-right"
                  id="timeline-chart-right"
                  timelineA={getTimeline()}
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
          </FlamegraphRenderer>
        </Box>
      </div>
      <Footer />
    </div>
  );
}

export default ComparisonDiffApp;
