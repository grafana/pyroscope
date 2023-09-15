/* eslint-disable import/first */
import 'react-dom';
import React from 'react';

import ReactFlot from 'react-flot';
import 'react-flot/flot/jquery.flot.time.min';
import '@pyroscope/components/TimelineChart/Selection.plugin';
import 'react-flot/flot/jquery.flot.crosshair';
import '@pyroscope/components/TimelineChart/TimelineChartPlugin';
import '@pyroscope/components/TimelineChart/Tooltip.plugin';
import '@pyroscope/components/TimelineChart/Annotations.plugin';
import '@pyroscope/components/TimelineChart/ContextMenu.plugin';
import '@pyroscope/components/TimelineChart/CrosshairSync.plugin';

interface TimelineChartProps {
  onSelect: (from: string, until: string) => void;
  className: string;
  ['data-testid']?: string;
}

class TimelineChart extends ReactFlot<TimelineChartProps> {
  componentDidMount() {
    this.draw();

    // TODO: use ref
    $(`#${this.props.id}`).bind('plotselected', (event, ranges) => {
      // Before invoking the onselect, we ensure the selection is valid

      if (ranges.xaxis == null) {
        // Invalid range, do nothing.
        return;
      }

      const { from, to } = ranges.xaxis;

      let fromSeconds = Math.round(from / 1000);
      let untilSeconds = Math.round(to / 1000);

      if (fromSeconds === untilSeconds) {
        // If the time ranges have a difference of zero seconds, we skip the zoom.
        return;
      }

      let morePointsThanPixels = false;
      let pointCount = 0;

      for (const dataRange of this.props.data) {
        if (
          morePointsThanPixels ||
          dataRange.data.length > event.currentTarget.clientWidth
        ) {
          // We can assume there are some points in the selection, so we will short circuit out
          morePointsThanPixels = true;
          break;
        }

        dataRange.data.forEach(
          (point: number[]) =>
            (pointCount += Number(point[0] > from && point[0] < to))
        );
      }

      if (!morePointsThanPixels && pointCount < 3) {
        // If we are zooming in and there are fewer than 3 points, we cancel the zoom.
        return;
      }

      this.props.onSelect(fromSeconds.toString(), untilSeconds.toString());
    });
  }

  componentDidUpdate() {
    this.draw();
  }

  componentWillReceiveProps() {}

  // copied directly from ReactFlot implementation
  // https://github.com/rodrigowirth/react-flot/blob/master/src/ReactFlot.jsx
  render() {
    const style = {
      height: this.props.height || '100px',
      width: this.props.width || '100%',
    };

    return (
      <div
        data-testid={this.props['data-testid']}
        className={this.props.className}
        id={this.props.id}
        style={style}
      />
    );
  }
}

export default TimelineChart;
