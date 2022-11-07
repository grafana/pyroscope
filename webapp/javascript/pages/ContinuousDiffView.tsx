import React, { useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import Box from '@webapp/ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
  fetchTagValues,
  selectQueries,
  selectTimelineSides,
} from '@webapp/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/flamegraph/src/FlamegraphRenderer';
import usePopulateLeftRightQuery from '@webapp/hooks/populateLeftRightQuery.hook';
import useTimelines, {
  leftColor,
  rightColor,
  selectionColor,
} from '@webapp/hooks/timeline.hook';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import useColorMode from '@webapp/hooks/colorMode.hook';
import useTags from '@webapp/hooks/tags.hook';
import Toolbar from '@webapp/components/Toolbar';
import TagsBar from '@webapp/components/TagsBar';
import TimelineChartWrapper from '@webapp/components/TimelineChart/TimelineChartWrapper';
import SyncTimelines from '@webapp/components/TimelineChart/SyncTimelines';
import useExportToFlamegraphDotCom from '@webapp/components/exportToFlamegraphDotCom.hook';
import { LoadingOverlay } from '@webapp/ui/LoadingOverlay';
import ExportData from '@webapp/components/ExportData';
import TimelineTitle from '@webapp/components/TimelineTitle';
import { isExportToFlamegraphDotComEnabled } from '@webapp/util/features';
import PageTitle from '@webapp/components/PageTitle';
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

  usePopulateLeftRightQuery();
  const { leftTags, rightTags } = useTags({ leftQuery, rightQuery });
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
          hideTagsBar
          onSelectedName={(query) => {
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
              timelineA={leftTimeline}
              timelineB={rightTimeline}
              onSelect={(from, until) => {
                dispatch(actions.setFromAndUntil({ from, until }));
              }}
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
              title={
                <TimelineTitle titleKey={diffView.profile?.metadata.units} />
              }
            />
            <SyncTimelines
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
              <TimelineTitle titleKey="baseline" color={leftColor} />
              <TagsBar
                query={leftQuery}
                tags={leftTags}
                onSetQuery={(q) => {
                  dispatch(actions.setLeftQuery(q));
                  if (leftQuery === q) {
                    dispatch(actions.refresh());
                  }
                }}
                onSelectedLabel={(label, query) => {
                  dispatch(fetchTagValues({ query, label }));
                }}
              />
              <TimelineChartWrapper
                data-testid="timeline-left"
                key="timeline-chart-left"
                id="timeline-chart-left"
                timelineA={leftTimeline}
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
              <TimelineTitle titleKey="comparison" color={rightColor} />
              <TagsBar
                query={rightQuery}
                tags={rightTags}
                onSetQuery={(q) => {
                  dispatch(actions.setRightQuery(q));
                  if (rightQuery === q) {
                    dispatch(actions.refresh());
                  }
                }}
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
            <TimelineTitle titleKey="diff" />
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
