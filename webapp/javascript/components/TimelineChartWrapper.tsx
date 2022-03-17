/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/no-did-update-set-state */
/* eslint-disable react/destructuring-assignment */
import React from 'react';
import { Timeline } from '@models/timeline';
import Color from 'color';
import TimelineChart from './TimelineChart';
import { formatAsOBject } from '../util/formatDate';
import styles from './TimelineChartWrapper.module.css';

interface TimelineData {
  data?: Timeline;
  color?: Color;
}

interface Marking {
  from: string;
  to: string;
  color: Color;
}

type TimelineChartWrapperProps = {
  /** the id attribute of the element float will use to apply to, it should be unique */
  id: string;

  ['data-testid']?: string;
  onSelect: (from: string, until: string) => void;
  format: 'lines' | 'bars';

  /** timelineA refers to the first (and maybe unique) timeline */
  timelineA: TimelineData;
  /** timelineB refers to the second timeline, useful for comparison view */
  timelineB?: TimelineData;

  /** refers to the highlighted selection */
  markings?: {
    left?: Marking;
    right?: Marking;
  };
};

class TimelineChartWrapper extends React.Component<
  TimelineChartWrapperProps,
  // TODO add type
  ShamefulAny
> {
  // eslint-disable-next-line react/static-property-placement
  static defaultProps = {
    format: 'bars',
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
        mode: 'x',
      },
      crosshair: {
        mode: 'x',
        color: '#C3170D',
        lineWidth: '1',
      },
      grid: {
        borderWidth: 1,
        hoverable: true,
      },
      yaxis: {
        show: false,
        min: 0,
      },
      points: {
        show: false,
      },
      lines: {
        show: false,
      },
      bars: {
        show: true,
      },
      xaxis: {
        mode: 'time',
        timezone: 'browser',
        reserveSpace: false,
      },
    };

    flotOptions = (() => {
      switch (props.format) {
        case 'lines': {
          return {
            ...flotOptions,
            lines: {
              show: true,
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
  }

  componentDidUpdate(prevProps: TimelineChartWrapperProps) {
    if (prevProps.markings !== this.props.markings) {
      const newFlotOptions = this.state.flotOptions;
      newFlotOptions.grid.markings = this.plotMarkings();
      this.setState({ flotOptions: newFlotOptions });
    }
  }

  plotMarkings = () => {
    const constructMarking = (m: Marking) => {
      const from = new Date(formatAsOBject(m.from)).getTime();
      const to = new Date(formatAsOBject(m.to)).getTime();

      // We make the sides thicker to indicate the boundary
      const boundary = { lineWidth: 3, color: m.color.rgb() };

      return [
        {
          xaxis: { from, to },
          color: m.color.rgb().alpha(0.35),
        },
        { ...boundary, xaxis: { from, to: from } },
        { ...boundary, xaxis: { from: to, to } },
      ];
    };

    const { markings } = this.props;

    if (markings) {
      return [
        markings.left && constructMarking(markings.left),
        markings.right && constructMarking(markings.right),
      ]
        .flat()
        .filter((a) => !!a);
    }

    return [];
  };

  render = () => {
    const { flotOptions } = this.state;
    const { id, timelineA } = this.props;
    let { timelineB } = this.props;

    const customFlotOptions = {
      ...flotOptions,
      xaxis: {
        ...flotOptions.xaxis,
        // In case there are few chunks left, then we'd like to add some margins to
        // both sides making it look more centers
        autoscaleMargin:
          timelineA.data && timelineA.data.samples.length > 3 ? null : 0.005,
      },
    };

    // If they are the same, skew the second one slightly so that they are both visible
    if (areTimelinesTheSame(timelineA, timelineB)) {
      // the factor is completely arbitrary, we use a positive number to skew above
      timelineB = skewTimeline(timelineB, 4);

      // check if both have a single value
      // if so, let's use bars
      // since we can't put a point when there's no data when using points
      if (timelineB && timelineB.data && timelineB.data.samples.length <= 1) {
        customFlotOptions.bars.show = true;

        // Also slightly skew to show them side by side
        timelineB.data.startTime += 0.01;
      } else {
        customFlotOptions.bars.show = false;
      }
    }

    const data = [
      timelineA &&
        timelineA.data && { ...timelineA, data: centerTimelineData(timelineA) },
      timelineB &&
        timelineB.data && { ...timelineB, data: centerTimelineData(timelineB) },
    ].filter((a) => !!a);

    return (
      <TimelineChart
        onSelect={this.props.onSelect}
        className={styles.wrapper}
        // eslint-disable-next-line react/destructuring-assignment
        data-testid={this.props['data-testid']}
        id={id}
        options={customFlotOptions}
        data={data}
        //        data={d}
        width="100%"
        height="100px"
      />
    );
  };
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

  if (dataA.samples.length !== dataB.samples.length) {
    return false;
  }

  const add = (acc: number, a: number) => acc + a;

  // TODO: actually check if they are the same
  // this is a very poor heuristic
  const sumA = dataA.samples.reduce(add, 0);
  const sumB = dataB.samples.reduce(add, 0);
  return sumA === sumB;
}
// Since profiling data is chuked by 10 seconds slices
// it's more user friendly to point a `center` of a data chunk
// as a bar rather than starting point, so we add 5 seconds to each chunk to 'center' it
function centerTimelineData(timelineData: TimelineData) {
  return timelineData.data
    ? decodeTimelineData(timelineData.data).map((x) => [
        x[0] + 5000,
        x[1] === 0 ? 0 : x[1] - 1,
      ])
    : [[]];
}

function decodeTimelineData(timeline: Timeline) {
  if (!timeline) {
    return [];
  }
  let time = timeline.startTime;
  return timeline.samples.map((x) => {
    const res = [time * 1000, x];
    time += timeline.durationDelta;
    return res;
  });
}

export default TimelineChartWrapper;
