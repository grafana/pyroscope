import React, { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@phlare/redux/hooks';
import Box from '@phlare/ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
  fetchTagValues,
  selectQueries,
  selectTimelineSides,
  selectAnnotationsOrDefault,
} from '@phlare/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import usePopulateLeftRightQuery from '@phlare/hooks/populateLeftRightQuery.hook';
import useTimelines, {
  leftColor,
  rightColor,
  selectionColor,
} from '@phlare/hooks/timeline.hook';
import useTimeZone from '@phlare/hooks/timeZone.hook';
import useColorMode from '@phlare/hooks/colorMode.hook';
import useTags from '@phlare/hooks/tags.hook';
import Toolbar from '@phlare/components/Toolbar';
import TagsBar from '@phlare/components/TagsBar';
import TimelineChartWrapper from '@phlare/components/TimelineChart/TimelineChartWrapper';
import SyncTimelines from '@phlare/components/TimelineChart/SyncTimelines';
import useExportToFlamegraphDotCom from '@phlare/components/exportToFlamegraphDotCom.hook';
import { LoadingOverlay } from '@phlare/ui/LoadingOverlay';
import ExportData from '@phlare/components/ExportData';
import ChartTitle from '@phlare/components/ChartTitle';
import { isExportToFlamegraphDotComEnabled } from '@phlare/util/features';
import PageTitle from '@phlare/components/PageTitle';
import { formatTitle } from './formatTitle';
import { isLoadingOrReloading } from './loading';

function ComparisonDiffApp() {
  const dispatch = useAppDispatch();
  const { colorMode } = useColorMode();
  const {
    diffView,
    refreshToken,
    maxNodes,
    leftFrom,
    rightFrom,
    leftUntil,
    rightUntil,
  } = useAppSelector(selectContinuousState);
  const { leftQuery, rightQuery } = useAppSelector(selectQueries);
  const annotations = useAppSelector(selectAnnotationsOrDefault('diffView'));

  usePopulateLeftRightQuery();
  const { leftTags, rightTags } = useTags();
  const { leftTimeline, rightTimeline } = useTimelines();

  const timelines = useAppSelector(selectTimelineSides);
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    diffView.profile
  );

  const { offset } = useTimeZone();
  const timezone = offset === 0 ? 'utc' : 'browser';

  const isLoading = isLoadingOrReloading([
    diffView.type,
    timelines.left.type,
    timelines.right.type,
  ]);

  useEffect(() => {
    if (rightQuery && leftQuery) {
      const fetchData = dispatch(
        fetchDiffView({
          leftQuery,
          leftFrom,
          leftUntil,

          rightQuery,
          rightFrom,
          rightUntil,
        })
      );
      return fetchData.abort;
    }
    return undefined;
  }, [
    dispatch,
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
      exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
      exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
    />
  );

  return (
    <div>
      <PageTitle title={formatTitle('Diff', leftQuery, rightQuery)} />
      <div className="main-wrapper">
        <Toolbar
          onSelectedApp={(query) => {
            dispatch(actions.setQuery(query));
          }}
        />
        <Box>
          <LoadingOverlay active={isLoading}>
            <TimelineChartWrapper
              data-testid="timeline-main"
              id="timeline-chart-diff"
              format="lines"
              height="125px"
              annotations={annotations}
              timelineA={leftTimeline}
              timelineB={rightTimeline}
              onSelect={(from, until) => {
                dispatch(actions.setFromAndUntil({ from, until }));
              }}
              syncCrosshairsWith={[
                'timeline-chart-left',
                'timeline-chart-right',
              ]}
              selection={{
                left: {
                  from: leftFrom,
                  to: leftUntil,
                  color: leftColor,
                  overlayColor: leftColor.alpha(0.3),
                },
                right: {
                  from: rightFrom,
                  to: rightUntil,
                  color: rightColor,
                  overlayColor: rightColor.alpha(0.3),
                },
              }}
              selectionType="double"
              timezone={timezone}
              title={<ChartTitle titleKey={diffView.profile?.metadata.units} />}
            />
            <SyncTimelines
              isDataLoading={isLoading}
              timeline={leftTimeline}
              leftSelection={{ from: leftFrom, to: leftUntil }}
              rightSelection={{ from: rightFrom, to: rightUntil }}
              onSync={(from, until) => {
                dispatch(actions.setFromAndUntil({ from, until }));
              }}
            />
          </LoadingOverlay>
        </Box>
        <div className="diff-instructions-wrapper">
          <Box className="diff-instructions-wrapper-side">
            <LoadingOverlay active={isLoading}>
              <ChartTitle titleKey="baseline" color={leftColor} />
              <TagsBar
                query={leftQuery}
                tags={leftTags}
                onRefresh={() => dispatch(actions.refresh())}
                onSetQuery={(q) => dispatch(actions.setLeftQuery(q))}
                onSelectedLabel={(label, query) => {
                  dispatch(fetchTagValues({ query, label }));
                }}
              />
              <TimelineChartWrapper
                data-testid="timeline-left"
                key="timeline-chart-left"
                id="timeline-chart-left"
                timelineA={leftTimeline}
                syncCrosshairsWith={[
                  'timeline-chart-diff',
                  'timeline-chart-right',
                ]}
                selectionWithHandler
                onSelect={(from, until) => {
                  dispatch(actions.setLeft({ from, until }));
                }}
                selection={{
                  left: {
                    from: leftFrom,
                    to: leftUntil,
                    color: selectionColor,
                    overlayColor: selectionColor.alpha(0.3),
                  },
                }}
                selectionType="single"
                timezone={timezone}
              />
            </LoadingOverlay>
          </Box>
          <Box className="diff-instructions-wrapper-side">
            <LoadingOverlay active={isLoading}>
              <ChartTitle titleKey="comparison" color={rightColor} />
              <TagsBar
                query={rightQuery}
                tags={rightTags}
                onRefresh={() => dispatch(actions.refresh())}
                onSetQuery={(q) => dispatch(actions.setRightQuery(q))}
                onSelectedLabel={(label, query) => {
                  dispatch(fetchTagValues({ query, label }));
                }}
              />
              <TimelineChartWrapper
                data-testid="timeline-right"
                key="timeline-chart-right"
                id="timeline-chart-right"
                selectionWithHandler
                timelineA={rightTimeline}
                syncCrosshairsWith={[
                  'timeline-chart-diff',
                  'timeline-chart-left',
                ]}
                onSelect={(from, until) => {
                  dispatch(actions.setRight({ from, until }));
                }}
                selection={{
                  right: {
                    from: rightFrom,
                    to: rightUntil,
                    color: selectionColor,
                    overlayColor: selectionColor.alpha(0.3),
                  },
                }}
                selectionType="single"
                timezone={timezone}
              />
            </LoadingOverlay>
          </Box>
        </div>
        <Box>
          <LoadingOverlay active={isLoading} spinnerPosition="baseline">
            <ChartTitle titleKey="diff" />
            <FlamegraphRenderer
              showCredit={false}
              profile={diffView.profile}
              ExportData={exportData}
              colorMode={colorMode}
            />
          </LoadingOverlay>
        </Box>
      </div>
    </div>
  );
}

export default ComparisonDiffApp;
