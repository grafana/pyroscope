import React, { useEffect } from 'react';

import { createTheme } from '@grafana/data';
import { FlameGraph } from '@grafana/flamegraph';
import { Button, Tooltip } from '@grafana/ui';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@pyroscope/ui/Box';
import {
  fetchDiffView,
  selectContinuousState,
  actions,
  fetchTagValues,
  selectQueries,
  selectTimelineSides,
  selectAnnotationsOrDefault,
  DiffView,
} from '@pyroscope/redux/reducers/continuous';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
import usePopulateLeftRightQuery from '@pyroscope/hooks/populateLeftRightQuery.hook';
import useTimelines, {
  leftColor,
  rightColor,
  selectionColor,
} from '@pyroscope/hooks/timeline.hook';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import useTags from '@pyroscope/hooks/tags.hook';
import Toolbar from '@pyroscope/components/Toolbar';
import TagsBar from '@pyroscope/components/TagsBar';
import TimelineChartWrapper from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import SyncTimelines from '@pyroscope/components/TimelineChart/SyncTimelines';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import { LoadingOverlay } from '@pyroscope/ui/LoadingOverlay';
import ExportData from '@pyroscope/components/ExportData';
import ChartTitle from '@pyroscope/components/ChartTitle';
import {
  isExportToFlamegraphDotComEnabled,
  isGrafanaFlamegraphEnabled,
} from '@pyroscope/util/features';
import PageTitle from '@pyroscope/components/PageTitle';
import { formatTitle } from './formatTitle';
import { isLoadingOrReloading } from './loading';
import { PageContentWrapper } from './layout';
import { flamebearerToDataFrameDTO } from '@pyroscope/util/flamebearer';

function ComparisonDiffApp() {
  const dispatch = useAppDispatch();
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

  return (
    <div>
      <PageTitle title={formatTitle('Diff', leftQuery, rightQuery)} />
      <PageContentWrapper>
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
            <FlamegraphContainer diffView={diffView} />
          </LoadingOverlay>
        </Box>
      </PageContentWrapper>
    </div>
  );
}

function FlamegraphContainer({ diffView }: { diffView: DiffView }) {
  const { colorMode } = useColorMode();
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(
    diffView.profile
  );

  if (isGrafanaFlamegraphEnabled) {
    const dataFrame = diffView.profile
      ? flamebearerToDataFrameDTO(
          diffView.profile.flamebearer.levels,
          diffView.profile.flamebearer.names,
          true
        )
      : undefined;

    const exportData = diffView.profile && (
      <ExportData
        flamebearer={diffView.profile}
        exportJSON
        exportPNG
        // disable this until we fix it
        //      exportHTML
        exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
        exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
        buttonEl={({ onClick }) => {
          return (
            <Tooltip content={'Export Data'}>
              <Button
                // Ugly hack to go around globally defined line height messing up sizing of the button.
                // Not sure why it happens even if everything is display: Block. To override it would
                // need changes in Flamegraph which would be weird so this seems relatively sensible.
                style={{ marginTop: -7 }}
                icon={'download-alt'}
                size={'sm'}
                variant={'secondary'}
                fill={'outline'}
                onClick={onClick}
              />
            </Tooltip>
          );
        }}
      />
    );

    return (
      <FlameGraph
        getTheme={() => createTheme({ colors: { mode: colorMode } })}
        data={dataFrame}
        extraHeaderElements={exportData}
      />
    );
  } else {
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
      <FlamegraphRenderer
        showCredit={false}
        profile={diffView.profile}
        ExportData={exportData}
        colorMode={colorMode}
      />
    );
  }
}

export default ComparisonDiffApp;
