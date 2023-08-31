import React, { useEffect } from 'react';
import 'react-dom';

import {
  createTheme,
  DataFrameDTO,
  FieldType,
  createDataFrame,
} from '@grafana/data';
import { FlameGraph } from '@grafana/flamegraph';
import { Button, Tooltip } from '@grafana/ui';

import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import Box from '@pyroscope/ui/Box';
import { FlamegraphRenderer } from '@pyroscope/legacy/flamegraph/FlamegraphRenderer';
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
import ExportData from '@pyroscope/components/ExportData';
import ChartTitle from '@pyroscope/components/ChartTitle';
import useExportToFlamegraphDotCom from '@pyroscope/components/exportToFlamegraphDotCom.hook';
import TagsBar from '@pyroscope/components/TagsBar';
import useTimeZone from '@pyroscope/hooks/timeZone.hook';
import PageTitle from '@pyroscope/components/PageTitle';
import { ContextMenuProps } from '@pyroscope/components/TimelineChart/ContextMenu.plugin';
import { getFormatter } from '@pyroscope/legacy/flamegraph/format/format';
import { LoadingOverlay } from '@pyroscope/ui/LoadingOverlay';
import { TooltipCallbackProps } from '@pyroscope/components/TimelineChart/Tooltip.plugin';
import { Profile } from '@pyroscope/legacy/models';
import {
  isExportToFlamegraphDotComEnabled,
  isAnnotationsEnabled,
} from '@pyroscope/util/features';
import useTags from '@pyroscope/hooks/tags.hook';
import {
  TimelineTooltip,
  TimelineTooltipProps,
} from '@pyroscope/components/TimelineTooltip';
import { formatTitle } from './formatTitle';
import ContextMenu from './continuous/contextMenu/ContextMenu';
import AddAnnotationMenuItem from './continuous/contextMenu/AddAnnotation.menuitem';
import { isLoadingOrReloading } from './loading';
import { PageContentWrapper } from './layout';

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

  const getRaw = () => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        return singleView.profile;
      }

      default: {
        return undefined;
      }
    }
  };
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(getRaw());

  const newFlamegraph = true;
  const flamegraphRenderer = (() => {
    switch (singleView.type) {
      case 'loaded':
      case 'reloading': {
        if (newFlamegraph) {
          const dataFrame = diffFlamebearerToDataFrameDTO(
            singleView.profile?.flamebearer.levels,
            singleView.profile?.flamebearer.names
          );
          return (
            <FlameGraph
              getTheme={() => createTheme({ colors: { mode: 'dark' } })}
              data={dataFrame}
              extraHeaderElements={
                <ExportData
                  flamebearer={singleView.profile}
                  exportPNG
                  exportJSON
                  exportPprof
                  exportHTML
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
              }
            />
          );
        }

        return (
          <FlamegraphRenderer
            showCredit={false}
            profile={singleView.profile}
            colorMode={colorMode}
            ExportData={
              <ExportData
                flamebearer={singleView.profile}
                exportPNG
                exportJSON
                exportPprof
                exportHTML
                exportFlamegraphDotCom={isExportToFlamegraphDotComEnabled}
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

        <Box>
          <LoadingOverlay active={isLoadingOrReloading([singleView.type])}>
            <TimelineChartWrapper
              timezone={offset === 0 ? 'utc' : 'browser'}
              data-testid="timeline-single"
              id="timeline-chart-single"
              timelineA={getTimeline()}
              onSelect={(from, until) =>
                dispatch(setDateRange({ from, until }))
              }
              height="125px"
              title={
                <ChartTitle
                  className="singleView-timeline-title"
                  titleKey={singleView?.profile?.metadata.units}
                />
              }
              annotations={annotations}
              selectionType="single"
              ContextMenu={contextMenu}
              onHoverDisplayTooltip={(data) =>
                createTooltip(query, data, singleView.profile)
              }
            />
          </LoadingOverlay>
        </Box>
        <Box>
          <LoadingOverlay
            spinnerPosition="baseline"
            active={isLoadingOrReloading([singleView.type])}
          >
            {flamegraphRenderer}
          </LoadingOverlay>
        </Box>
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

function getNodes(level: number[], names: string[]) {
  const nodes = [];
  for (let i = 0; i < level.length; i += 4) {
    nodes.push({
      level: 0,
      label: names[level[i + 3]],
      val: level[i + 1],
      self: level[i + 2],
      offset: level[i],
      children: [],
    });
  }
  return nodes;
}

function diffFlamebearerToDataFrameDTO(levels: number[][], names: string[]) {
  const nodeLevels: any[][] = [];
  for (let i = 0; i < levels.length; i++) {
    nodeLevels[i] = [];
    for (const node of getNodes(levels[i], names)) {
      node.level = i;
      nodeLevels[i].push(node);
      if (i > 0) {
        const prevNodesInLevel = nodeLevels[i].slice(0, -1);
        const currentNodeStart =
          prevNodesInLevel.reduce(
            (acc: number, n: any) => n.offset + n.val + acc,
            0
          ) + node.offset;

        const prevLevel = nodeLevels[i - 1];
        let prevLevelOffset = 0;
        for (const prevLevelNode of prevLevel) {
          const parentNodeStart = prevLevelOffset + prevLevelNode.offset;
          const parentNodeEnd = parentNodeStart + prevLevelNode.val;

          if (
            parentNodeStart <= currentNodeStart &&
            parentNodeEnd > currentNodeStart
          ) {
            prevLevelNode.children.push(node);
            break;
          } else {
            prevLevelOffset += prevLevelNode.offset + prevLevelNode.val;
          }
        }
      }
    }
  }

  const root = nodeLevels[0][0];
  const stack = [root];

  const labelValues = [];
  const levelValues = [];
  const selfValues = [];
  const valueValues = [];

  while (stack.length) {
    const node = stack.shift();
    labelValues.push(node.label);
    levelValues.push(node.level);
    selfValues.push(node.self);
    valueValues.push(node.val);
    stack.unshift(...node.children);
  }

  const frame: DataFrameDTO = {
    name: 'response',
    meta: { preferredVisualisationType: 'flamegraph' },
    fields: [
      { name: 'level', values: levelValues },
      { name: 'label', values: labelValues, type: FieldType.string },
      { name: 'self', values: selfValues },
      { name: 'value', values: valueValues },
    ],
  };

  return createDataFrame(frame);
}

export default ContinuousSingleView;
