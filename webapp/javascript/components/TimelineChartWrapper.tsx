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
  color: string;
}

interface Marking {
  from: string;
  to: string;
  color: Color;
}

type TimelineChartWrapperProps = {
  /** the id attribute of the element float will use to apply to, it should be unique */
  id: string;
  timeline: TimelineData[];
  //  timeline?: Timeline[];
  ['data-testid']?: string;
  color?: string;
  onSelect: (from: string, until: string) => void;
  format: 'lines' | 'bars';

  left: TimelineData;
  right?: TimelineData;

  /** refers to the highlighted selection */
  markings?: {
    left?: Marking;
    right?: Marking;
  };
} /** it will use this info to color the markins */ & (
  | {
      viewSide: 'left';
      leftFrom: string;
      leftUntil: string;

      rightFrom?: string;
      rightUntil?: string;
    }
  | {
      viewSide: 'right';
      rightFrom: string;
      rightUntil: string;

      leftFrom?: string;
      leftUntil?: string;
    }
  | { viewSide: 'none' }
  | {
      viewSide: 'both';
      rightFrom: string;
      rightUntil: string;
      leftFrom?: string;
      leftUntil?: string;
    }
);

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
        //          radius: 0.1,
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
    const { timeline, id, viewSide } = this.props;
    const { flotOptions } = this.state;

    // Since profiling data is chuked by 10 seconds slices
    // it's more user friendly to point a `center` of a data chunk
    // as a bar rather than starting point, so we add 5 seconds to each chunk to 'center' it
    //    const flotData = timeline
    //      ? [
    //          decodeTimelineData(timeline).map((x) => [
    //            x[0] + 5000,
    //            x[1] === 0 ? null : x[1] - 1,
    //          ]),
    //        ]
    //      : [];
    //
    // In case there are few chunks left, then we'd like to add some margins to
    // both sides making it look more centers
    const customFlotOptions = {
      ...flotOptions,
      xaxis: {
        ...flotOptions.xaxis,
        //        autoscaleMargin: flotData[0] && flotData[0].length > 3 ? null : 0.005,
      },
    };
    customFlotOptions.grid.markings = this.plotMarkings();

    // TODO: render something
    if (!timeline || timeline.filter((a) => !!a).length <= 0) {
      return null;
    }

    // let's only act on explicit data
    // otherwise the SideBySide plugin may not work properly
    const filtered = timeline.filter((a) => a?.data?.samples.length > 0);

    const copy = JSON.parse(JSON.stringify(filtered));

    // If they are the same, skew the second one slightly so that they are both visible
    // Skew the second one so that they are visible
    if (copy.length > 1 && copy[1].data && copy[1].data.samples) {
      const newSamples = copy[1].data.samples.map((a) => {
        // TODO: figure out by how much to skew
        return a - 5;
      });

      copy[1].data.samples = newSamples;
    }

    const d = copy.map((a, i) => {
      return {
        //        color: this.props.color ? this.props.color : null,
        color: a.color,
        // Since profiling data is chuked by 10 seconds slices
        // it's more user friendly to point a `center` of a data chunk
        // as a bar rather than starting point, so we add 5 seconds to each chunk to 'center' it
        data: a
          ? decodeTimelineData(a.data).map((x) => [
              x[0] + 5000,
              x[1] === 0 ? 0 : x[1] - 1,
            ])
          : [[]],
      };
    });

    return (
      <TimelineChart
        onSelect={this.props.onSelect}
        className={styles.wrapper}
        // eslint-disable-next-line react/destructuring-assignment
        data-testid={this.props['data-testid']}
        id={id}
        options={customFlotOptions}
        viewSide={viewSide}
        //        data={msData}
        data={d}
        width="100%"
        height="100px"
      />
    );
  };
}

function decodeTimelineData(timelineData: Timeline) {
  if (!timelineData) {
    return [];
  }
  let time = timelineData.startTime;
  return timelineData.samples.map((x) => {
    const res = [time * 1000, x];
    time += timelineData.durationDelta;
    return res;
  });
}

export default TimelineChartWrapper;
