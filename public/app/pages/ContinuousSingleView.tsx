import React, { useEffect, useRef, useState } from 'react';
import 'react-dom';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import {
  fetchSingleView,
  setQuery,
  selectQueries,
  setDateRange,
  selectAnnotationsOrDefault,
  addAnnotation,
  actions,
  fetchTagValues,
} from '@pyroscope/redux/reducers/continuous';
import useColorMode from '@pyroscope/hooks/colorMode.hook';
import TimelineChartWrapper from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import Toolbar from '@pyroscope/components/Toolbar';
import ChartTitle from '@pyroscope/components/ChartTitle';
import TagsBar from '@pyroscope/components/TagsBar';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import PageTitle from '@pyroscope/components/PageTitle';
import { ContextMenuProps } from '@pyroscope/components/TimelineChart/ContextMenu.plugin';
import { getFormatter } from '@pyroscope/legacy/flamegraph/format/format';
import { TooltipCallbackProps } from '@pyroscope/components/TimelineChart/Tooltip.plugin';
import { Profile } from '@pyroscope/legacy/models';
import { isAnnotationsEnabled } from '@pyroscope/util/features';
import useTags from '@pyroscope/hooks/tags.hook';
import {
  TimelineTooltip,
  TimelineTooltipProps,
} from '@pyroscope/components/TimelineTooltip';
import { formatTitle } from './formatTitle';
import ContextMenu from './continuous/contextMenu/ContextMenu';
import AddAnnotationMenuItem from './continuous/contextMenu/AddAnnotation.menuitem';
import { isLoadingOrReloading } from './loading';
import { Panel } from '@pyroscope/components/Panel';
import { PageContentWrapper } from '@pyroscope/pages/PageContentWrapper';
import {
  DrawerState,
  FlameGraphWrapper,
} from '@pyroscope/components/FlameGraphWrapper';
import { formatAsOBject } from '@pyroscope/util/formatDate';
import styles from './ContinuousSingleView.module.scss';
import { Box, Card, IconButton, InlineField, InlineLabel } from '@grafana/ui';
import Code from '@pyroscope/components/Code';

function ContinuousSingleView() {
  const dispatch = useAppDispatch();
  const { offset } = useTimeZone();
  const { colorMode } = useColorMode();

  const { query } = useAppSelector(selectQueries);
  const tags = useTags().regularTags;
  const { from, until, refreshToken, maxNodes } = useAppSelector(
    (state) => state.continuous
  );

  const { singleView } = useAppSelector((state) => state.continuous);
  const annotations = useAppSelector(selectAnnotationsOrDefault('singleView'));

  useEffect(() => {
    if (from && until && query && maxNodes) {
      const fetchData = dispatch(fetchSingleView(null));
      return () => fetchData.abort('cancel');
    }
    return undefined;
  }, [from, until, query, refreshToken, maxNodes, dispatch]);

  const start: number = formatAsOBject(from).getTime();
  const end: number = formatAsOBject(until).getTime();

  const [drawerState, setDrawerState] = useState<DrawerState | undefined>(
    undefined
  );

  const flamegraphRenderer = (() => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return (
          <FlameGraphWrapper
            profile={singleView.profile}
            query={query}
            start={start}
            end={end}
            setDrawerState={setDrawerState}
          />
        );
      }

      default: {
        return 'Loading';
      }
    }
  })();

  const getTimeline = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return {
          data: singleView.timeline,
          color: colorMode === 'light' ? '#3b78e7' : undefined,
        };
      }

      default: {
        return {
          data: undefined,
        };
      }
    }
  };

  const contextMenu = (props: ContextMenuProps) => {
    if (!isAnnotationsEnabled) {
      return null;
    }

    const { click, timestamp, containerEl } = props;

    if (!click) {
      return null;
    }

    return (
      <ContextMenu position={props?.click}>
        <AddAnnotationMenuItem
          container={containerEl}
          popoverAnchorPoint={{ x: click.pageX, y: click.pageY }}
          timestamp={timestamp}
          timezone={offset === 0 ? 'utc' : 'browser'}
          onCreateAnnotation={(content) => {
            dispatch(
              addAnnotation({
                appName: query,
                timestamp,
                content,
              })
            );
          }}
        />
      </ContextMenu>
    );
  };

  return (
    <div>
      <PageTitle title={formatTitle('Single', query)} />
      <PageContentWrapper>
        <Toolbar
          onSelectedApp={(query) => {
            dispatch(setQuery(query));
          }}
        />
        <TagsBar
          query={query}
          tags={tags}
          onRefresh={() => dispatch(actions.refresh())}
          onSetQuery={(q) => dispatch(actions.setQuery(q))}
          onSelectedLabel={(label, query) => {
            dispatch(fetchTagValues({ query, label }));
          }}
        />

        <Panel
          isLoading={isLoadingOrReloading([singleView.type])}
          title={
            <ChartTitle
              className="singleView-timeline-title"
              titleKey={singleView?.profile?.metadata.name as any}
            />
          }
        >
          <TimelineChartWrapper
            timezone={offset === 0 ? 'utc' : 'browser'}
            data-testid="timeline-single"
            id="timeline-chart-single"
            timelineA={getTimeline()}
            onSelect={(from, until) => dispatch(setDateRange({ from, until }))}
            height="125px"
            annotations={annotations}
            selectionType="single"
            ContextMenu={contextMenu}
            onHoverDisplayTooltip={(data) =>
              createTooltip(query, data, singleView.profile)
            }
          />
        </Panel>
        <div className={styles.container}>
          <Panel
            isLoading={isLoadingOrReloading([singleView.type])}
            className={styles.flamegraphContainer}
          >
            {flamegraphRenderer}
          </Panel>
          {drawerState && (
            <Panel
              isLoading={false}
              className={styles.codeContainer}
              title="Function Details"
              headerActions={
                <IconButton
                  name="times-circle"
                  variant="secondary"
                  aria-label="close"
                />
              }
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <InlineLabel width="auto"> Repository</InlineLabel>
                <span>{drawerState.repository}</span>
              </div>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  paddingTop: '0.5rem',
                }}
              >
                <InlineLabel width="auto"> Commit</InlineLabel>
                <span>{drawerState.gitRef}</span>
              </div>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  paddingTop: '0.5rem',
                }}
              >
                <InlineLabel width="auto">File</InlineLabel>
                <span>{drawerState.filename}</span>
              </div>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  paddingTop: '0.5rem',
                }}
              >
                <InlineLabel width="auto">Function name</InlineLabel>
                <span>{drawerState.functionName}</span>
              </div>

              <div style={{ paddingTop: '0.5rem' }}>
                <InlineLabel
                  style={{
                    marginBottom: '0.5rem',
                  }}
                >
                  Breakdown per lines:
                </InlineLabel>
              </div>
              <Code
                lines={drawerState.code.lines}
                unit={drawerState.code.unit}
              ></Code>
            </Panel>
          )}
        </div>
      </PageContentWrapper>
    </div>
  );
}

function createTooltip(
  query: string,
  data: TooltipCallbackProps,
  profile?: Profile
) {
  if (!profile) {
    return null;
  }

  const values = prepareTimelineTooltipContent(profile, query, data);

  if (values.length <= 0) {
    return null;
  }

  return <TimelineTooltip timeLabel={data.timeLabel} items={values} />;
}

// Converts data from TimelineChartWrapper into TimelineTooltip
function prepareTimelineTooltipContent(
  profile: Profile,
  query: string,
  data: TooltipCallbackProps
): TimelineTooltipProps['items'] {
  const formatter = getFormatter(
    profile.flamebearer.numTicks,
    profile.metadata.sampleRate,
    profile.metadata.units
  );

  // Filter non empty values
  return (
    data.values
      .map((a) => {
        return {
          label: query,
          // TODO: horrible API
          value: a?.closest?.[1],
        };
      })
      // Sometimes closest is null
      .filter((a) => {
        return a.value;
      })
      .map((a) => {
        return {
          ...a,
          value: formatter.format(a.value, profile.metadata.sampleRate, true),
        };
      })
  );
}

export default ContinuousSingleView;
