/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/no-did-update-set-state */
/* eslint-disable react/destructuring-assignment */
import React, { ReactNode } from 'react';
import Color from 'color';
import type { Group } from '@pyroscope/legacy/models';
import type { Timeline } from '@pyroscope/models/timeline';
import { Annotation } from '@pyroscope/models/annotation';
import Legend from '@pyroscope/pages/tagExplorer/components/Legend';
import type { TooltipCallbackProps } from '@pyroscope/components/TimelineChart/Tooltip.plugin';
import TooltipWrapper from '@pyroscope/components/TimelineChart/TooltipWrapper';
import type { TooltipWrapperProps } from '@pyroscope/components/TimelineChart/TooltipWrapper';
import TimelineChart from '@pyroscope/components/TimelineChart/TimelineChart';
import { ContextMenuProps } from '@pyroscope/components/TimelineChart/ContextMenu.plugin';
import {
  markingsFromSelection,
  ANNOTATION_COLOR,
} from '@pyroscope/components/TimelineChart/markings';
import { centerTimelineData } from '@pyroscope/components/TimelineChart/centerTimelineData';
import styles from './TimelineChartWrapper.module.css';

export interface TimelineGroupData {
  data: Group;
  tagName: string;
  color?: Color;
}

export interface TimelineData {
  data?: Timeline;
  color?: string;
}

export interface Selection {
  from: string;
  to: string;
  color: Color;
  overlayColor: Color;
}

type SingleDataProps = {
  /** used to display at max 2 time series */
  mode: 'singles';
  /** timelineA refers to the first (and maybe unique) timeline */
  timelineA: TimelineData;
  /** timelineB refers to the second timeline, useful for comparison view */
  timelineB?: TimelineData;
};

// Used in Tag Explorer
type MultipleDataProps = {
  /** used when displaying multiple time series. original use case is for tag explorer */
  mode: 'multiple';
  /** timelineGroups refers to group of timelines, useful for explore view */
  timelineGroups: TimelineGroupData[];
  /** if there is active group, the other groups should "dim" themselves */
  activeGroup: string;
  /** show or hide legend */
  showTagsLegend: boolean;
  /** to set active tagValue using <Legend /> */
  handleGroupByTagValueChange: (groupByTagValue: string) => void;
};

type TimelineDataProps = SingleDataProps | MultipleDataProps;

type TimelineChartWrapperProps = TimelineDataProps & {
  /** the id attribute of the element float will use to apply to, it should be globally unique */
  id: string;

  ['data-testid']?: string;
  onSelect: (from: string, until: string) => void;
  format: 'lines' | 'bars';

  height?: string;

  /** refers to the highlighted selection */
  selection?: {
    left?: Selection;
    right?: Selection;
  };

  timezone: 'browser' | 'utc';
  title?: ReactNode;

  /** whether to show a selection with grabbable handle
   * ATTENTION: it only works with a single selection */
  selectionWithHandler?: boolean;

  /** selection type 'single' => gray selection, 'double' => color selection */
  selectionType: 'single' | 'double';
  onHoverDisplayTooltip?: React.FC<TooltipCallbackProps>;

  /** list of annotations timestamp, to be rendered as markings */
  annotations?: Annotation[];

  /** What element to render when clicking */
  ContextMenu?: (props: ContextMenuProps) => React.ReactNode;

  /** The list of timeline IDs (flotjs component) to sync the crosshair with */
  syncCrosshairsWith?: string[];
};

class TimelineChartWrapper extends React.Component<
  TimelineChartWrapperProps,
  // TODO add type
  ShamefulAny
> {
  // eslint-disable-next-line react/static-property-placement
  static defaultProps = {
    format: 'bars',
    mode: 'singles',
    timezone: 'browser',
    height: '100px',
  };

  constructor(props: TimelineChartWrapperProps) {
    super(props);

    let flotOptions = {
      margin: {
        top: 0,
        left: 0,
        bottom: 0,
        right: 0,
      },
      selection: {
        selectionWithHandler: props.selectionWithHandler || false,
        mode: 'x',
        // custom selection works for 'single' selection type,
        // 'double' selection works in old fashion way
        // we use different props to customize selection appearance
        selectionType: props.selectionType,
        overlayColor:
          props.selectionType === 'double'
            ? undefined
            : props?.selection?.right?.overlayColor ||
              props?.selection?.left?.overlayColor,
        boundaryColor:
          props.selectionType === 'double'
            ? undefined
            : props?.selection?.right?.color || props?.selection?.left?.color,
      },
      crosshair: {
        mode: 'x',
        color: '#C3170D',
        lineWidth: '1',
      },
      grid: {
        borderWidth: 1, // outside border of the timelines
        hoverable: true,

        // For the contextMenu plugin to work. From the docs:
        // > If you set “clickable” to true, the plot will listen for click events
        //   on the plot area and fire a “plotclick” event on the placeholder with
        //   a position and a nearby data item object as parameters.
        clickable: true,
      },
      annotations: [],
      syncCrosshairsWith: [],
      yaxis: {
        show: false,
        min: 0,
      },
      points: {
        show: false,
        symbol: () => {}, // function that draw points on the chart
      },
      lines: {
        show: false,
      },
      bars: {
        show: true,
      },
      xaxis: {
        mode: 'time',
        timezone: props.timezone,
        reserveSpace: false,
        // according to https://github.com/flot/flot/blob/master/API.md#customizing-the-axes
        minTickSize: [3, 'second'],
      },
    };

    flotOptions = (() => {
      switch (props.format) {
        case 'lines': {
          return {
            ...flotOptions,
            lines: {
              show: true,
              lineWidth: 0.8,
            },
            bars: {
              show: false,
            },
          };
        }

        case 'bars': {
          return {
            ...flotOptions,
            bars: {
              show: true,
            },
            lines: {
              show: false,
            },
          };
        }
        default: {
          throw new Error(`Invalid format: '${props.format}'`);
        }
      }
    })();

    this.state = { flotOptions };
    this.state.flotOptions.grid.markings = this.plotMarkings();
    this.state.flotOptions.annotations = this.composeAnnotationsList();
  }

  // TODO: this only seems to sync props back into the state, which seems unnecessary
  componentDidUpdate(prevProps: TimelineChartWrapperProps) {
    if (
      prevProps.selection !== this.props.selection ||
      prevProps.annotations !== this.props.annotations ||
      prevProps.syncCrosshairsWith !== this.props.syncCrosshairsWith
    ) {
      const newFlotOptions = this.state.flotOptions;
      newFlotOptions.grid.markings = this.plotMarkings();
      newFlotOptions.annotations = this.composeAnnotationsList();
      newFlotOptions.syncCrosshairsWith = this.props.syncCrosshairsWith;

      this.setState({ flotOptions: newFlotOptions });
    }
  }

  composeAnnotationsList = () => {
    return Array.isArray(this.props.annotations)
      ? this.props.annotations?.map((a) => ({
          timestamp: a.timestamp,
          content: a.content,
          type: 'message',
          color: ANNOTATION_COLOR,
        }))
      : [];
  };

  plotMarkings = () => {
    const selectionMarkings = markingsFromSelection(
      this.props.selectionType,
      this.props.selection?.left,
      this.props.selection?.right
    );

    return [...selectionMarkings];
  };

  setOnHoverDisplayTooltip = (
    data: TooltipWrapperProps & TooltipCallbackProps
  ) => {
    const tooltipContent = [];

    const TooltipBody: React.FC<TooltipCallbackProps> | undefined =
      this.props?.onHoverDisplayTooltip;

    if (TooltipBody) {
      tooltipContent.push(
        <TooltipBody
          key="explore-body"
          values={data.values}
          timeLabel={data.timeLabel}
        />
      );
    }

    if (tooltipContent.length) {
      return (
        <TooltipWrapper
          align={data.align}
          pageY={data.pageY}
          pageX={data.pageX}
        >
          {tooltipContent.map((tooltipBody) => tooltipBody)}
        </TooltipWrapper>
      );
    }

    return null;
  };

  renderMultiple = (props: MultipleDataProps) => {
    const { flotOptions } = this.state;
    const { timelineGroups, activeGroup, showTagsLegend } = props;
    const { timezone } = this.props;

    // TODO: unify with renderSingle
    const onHoverDisplayTooltip = (
      data: TooltipWrapperProps & TooltipCallbackProps
    ) => this.setOnHoverDisplayTooltip(data);

    const customFlotOptions = {
      ...flotOptions,
      onHoverDisplayTooltip,
      ContextMenu: this.props.ContextMenu,
      xaxis: { ...flotOptions.xaxis, autoscaleMargin: null, timezone },
      wrapperId: this.props.id,
    };

    const centeredTimelineGroups = timelineGroups.map(
      ({ data, color, tagName }) => {
        return {
          data: centerTimelineData({ data }),
          tagName,
          color:
            activeGroup && activeGroup !== tagName ? color?.fade(0.75) : color,
        };
      }
    );

    return (
      <>
        {this.timelineChart(centeredTimelineGroups, customFlotOptions)}
        {showTagsLegend && (
          <Legend
            activeGroup={activeGroup}
            groups={timelineGroups}
            handleGroupByTagValueChange={props.handleGroupByTagValueChange}
          />
        )}
      </>
    );
  };

  renderSingle = (props: SingleDataProps) => {
    const { flotOptions } = this.state;
    const { timelineA } = props;
    let { timelineB } = props;
    const { timezone, title } = this.props;

    // TODO deep copy
    timelineB = timelineB ? JSON.parse(JSON.stringify(timelineB)) : undefined;

    // TODO: unify with renderMultiple
    const onHoverDisplayTooltip = (
      data: TooltipWrapperProps & TooltipCallbackProps
    ) => this.setOnHoverDisplayTooltip(data);

    const customFlotOptions = {
      ...flotOptions,
      onHoverDisplayTooltip,
      ContextMenu: this.props.ContextMenu,
      wrapperId: this.props.id,
      xaxis: {
        ...flotOptions.xaxis,
        // In case there are few chunks left, then we'd like to add some margins to
        // both sides making it look more centers
        autoscaleMargin:
          timelineA?.data && timelineA.data.samples.length > 3 ? null : 0.005,
        timezone,
      },
    };

    // Since this may be overwritten, we always need to set it up correctly
    if (timelineA && timelineB) {
      customFlotOptions.bars.show = false;
    } else {
      customFlotOptions.bars.show = true;
    }

    // If they are the same, skew the second one slightly so that they are both visible
    if (areTimelinesTheSame(timelineA, timelineB)) {
      // the factor is completely arbitrary, we use a positive number to skew above
      timelineB = skewTimeline(timelineB, 4);
    }

    if (isSingleDatapoint(timelineA, timelineB)) {
      // check if both have a single value
      // if so, let's use bars
      // since we can't put a point when there's no data when using points
      if (timelineB && timelineB.data && timelineB.data.samples.length <= 1) {
        customFlotOptions.bars.show = true;

        // Also slightly skew to show them side by side
        timelineB.data.startTime += 0.01;
      }
    }

    const data = [
      timelineA &&
        timelineA.data && {
          ...timelineA,
          data: centerTimelineData(timelineA),
        },
      timelineB &&
        timelineB.data && { ...timelineB, data: centerTimelineData(timelineB) },
    ].filter((a) => !!a);

    return (
      <>
        {title}
        {this.timelineChart(data, customFlotOptions)}
      </>
    );
  };

  timelineChart = (
    data: Array<{ data: number[][]; color?: string | Color } | undefined>,
    customFlotOptions: ShamefulAny
  ) => {
    return (
      <TimelineChart
        onSelect={this.props.onSelect}
        className={styles.wrapper}
        data-testid={this.props['data-testid']}
        id={this.props.id}
        options={customFlotOptions}
        data={data}
        width="100%"
        height={this.props.height}
      />
    );
  };

  render = () => {
    if (this.props.mode === 'multiple') {
      return this.renderMultiple(this.props);
    }
    return this.renderSingle(this.props);
  };
}

function isSingleDatapoint(timelineA: TimelineData, timelineB?: TimelineData) {
  const aIsSingle = timelineA.data && timelineA.data.samples.length <= 1;
  if (!aIsSingle) {
    return false;
  }

  if (timelineB && timelineB.data) {
    return timelineB.data.samples.length <= 1;
  }

  return true;
}

function skewTimeline(
  timeline: TimelineData | undefined,
  factor: number
): TimelineData | undefined {
  if (!timeline) {
    return undefined;
  }

  // TODO: deep copy
  const copy = JSON.parse(JSON.stringify(timeline)) as typeof timeline;

  if (copy && copy.data) {
    let min = copy.data.samples[0];
    let max = copy.data.samples[0];

    for (let i = 0; i < copy.data.samples.length; i += 1) {
      const b = copy.data.samples[i];

      if (b < min) {
        min = b;
      }
      if (b > max) {
        max = b;
      }
    }

    const height = 100; // px
    const skew = (max - min) / height;

    if (copy.data) {
      copy.data.samples = copy.data.samples.map((a) => {
        // We don't want to skew negative values, since users are expecting an absent value
        if (a <= 0) {
          return 0;
        }

        // 4 is completely arbitrary, it was eyeballed
        return a + skew * factor;
      });
    }
  }

  return copy;
}

function areTimelinesTheSame(
  timelineA: TimelineData,
  timelineB?: TimelineData
) {
  if (!timelineA || !timelineB) {
    // for all purposes let's consider two empty timelines the same
    // since we want to transform them
    return false;
  }
  const dataA = timelineA.data;
  const dataB = timelineB.data;

  if (!dataA || !dataB) {
    return false;
  }

  // Find the biggest one
  const biggest = dataA.samples.length > dataB.samples.length ? dataA : dataB;
  const smallest = dataA.samples.length < dataB.samples.length ? dataA : dataB;

  const map = new Map(biggest.samples.map((a) => [a, true]));

  return smallest.samples.every((a) => map.has(a));
}

export default TimelineChartWrapper;
