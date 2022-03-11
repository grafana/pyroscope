/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/no-did-update-set-state */
/* eslint-disable react/destructuring-assignment */
import React from 'react';
import { Timeline } from '@models/timeline';
import TimelineChart from './TimelineChart';
import { formatAsOBject } from '../util/formatDate';
import styles from './TimelineChartWrapper.module.css';

type TimelineChartWrapperProps = {
  /** the id attribute of the element float will use to apply to, it should be unique */
  id: string;
  timeline?: Timeline;
  onSelect: (from: string, until: string) => void;
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
  any
> {
  constructor(props: TimelineChartWrapperProps) {
    super(props);

    this.state = {
      flotOptions: {
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
          radius: 0.1,
        },
        lines: {
          show: false,
          steps: true,
          lineWidth: 1.0,
        },
        bars: {
          show: true,
          fill: true,
        },
        xaxis: {
          mode: 'time',
          timezone: 'browser',
          reserveSpace: false,
        },
      },
    };

    this.state.flotOptions.grid.markings = this.plotMarkings();
  }

  componentDidUpdate(prevProps) {
    if (this.props.viewSide === 'none') return;

    if (
      prevProps.leftFrom !== this.props.leftFrom ||
      prevProps.leftUntil !== this.props.leftUntil ||
      prevProps.rightFrom !== this.props.rightFrom ||
      prevProps.rightUntil !== this.props.rightUntil
    ) {
      const newFlotOptions = this.state.flotOptions;
      newFlotOptions.grid.markings = this.plotMarkings();
      this.setState({ flotOptions: newFlotOptions });
    }
  }

  plotMarkings = () => {
    const { viewSide } = this.props;
    if (viewSide === 'none' || !viewSide) {
      return [];
    }

    const nonActiveBorder = 0.2;
    const nonActiveBackground = 0.09;

    const leftMarkings = (() => {
      if (!this.props.leftFrom || !this.props.leftUntil) {
        return [];
      }

      const leftFromInt = new Date(
        formatAsOBject(this.props.leftFrom)
      ).getTime();
      const leftUntilInt = new Date(
        formatAsOBject(this.props.leftUntil)
      ).getTime();

      return [
        {
          xaxis: {
            from: leftFromInt,
            to: leftUntilInt,
          },
          color:
            viewSide === 'left'
              ? 'rgba(200, 102, 204, 0.35)'
              : `rgba(255, 102, 204, ${nonActiveBackground})`,
        },
        {
          color:
            viewSide === 'left'
              ? 'rgba(200, 102, 204, 1)'
              : `rgba(255, 102, 204, ${nonActiveBorder})`,
          lineWidth: 3,
          xaxis: { from: leftFromInt, to: leftFromInt },
        },
        {
          color:
            viewSide === 'left'
              ? 'rgba(200, 102, 204, 1)'
              : `rgba(255, 102, 204, ${nonActiveBorder})`,
          lineWidth: 3,
          xaxis: { from: leftUntilInt, to: leftUntilInt },
        },
      ];
    })();

    const rightMarkings = (() => {
      if (!this.props.rightUntil || !this.props.rightFrom) {
        return [];
      }

      const rightFromInt = new Date(
        formatAsOBject(this.props.rightFrom)
      ).getTime();
      const rightUntilInt = new Date(
        formatAsOBject(this.props.rightUntil)
      ).getTime();

      return [
        {
          xaxis: {
            from: rightFromInt,
            to: rightUntilInt,
          },
          color:
            viewSide === 'right'
              ? 'rgba(19, 152, 246, 0.35)'
              : `rgba(19, 152, 246, ${nonActiveBackground})`,
        },
        {
          color:
            viewSide === 'right'
              ? 'rgba(19, 152, 246, 1)'
              : `rgba(19, 152, 246, ${nonActiveBorder})`,
          lineWidth: 3,
          xaxis: { from: rightFromInt, to: rightFromInt },
        },
        {
          color:
            viewSide === 'right'
              ? 'rgba(19, 152, 246, 1)'
              : `rgba(19, 152, 246, ${nonActiveBorder})`,
          lineWidth: 3,
          xaxis: { from: rightUntilInt, to: rightUntilInt },
        },
      ];
    })();

    return leftMarkings.concat(rightMarkings);
  };

  render = () => {
    const { timeline, id, viewSide } = this.props;
    const { flotOptions } = this.state;

    // Since profiling data is chuked by 10 seconds slices
    // it's more user friendly to point a `center` of a data chunk
    // as a bar rather than starting point, so we add 5 seconds to each chunk to 'center' it
    const flotData = timeline
      ? [
          decodeTimelineData(timeline).map((x) => [
            x[0] + 5000,
            x[1] === 0 ? null : x[1] - 1,
          ]),
        ]
      : [];

    // In case there are few chunks left, then we'd like to add some margins to
    // both sides making it look more centers
    const customFlotOptions = {
      ...flotOptions,
      xaxis: {
        ...flotOptions.xaxis,
        autoscaleMargin: flotData[0] && flotData[0].length > 3 ? null : 0.005,
      },
    };

    return (
      <TimelineChart
        onSelect={this.props.onSelect}
        className={styles.wrapper}
        // eslint-disable-next-line react/destructuring-assignment
        data-testid={this.props['data-testid']}
        id={id}
        options={customFlotOptions}
        viewSide={viewSide}
        data={flotData}
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
