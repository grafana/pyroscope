// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck
import 'react-dom';
import React from 'react';

import ReactFlot from 'react-flot';
import 'react-flot/flot/jquery.flot.time.min';
import 'react-flot/flot/jquery.flot.selection.min';
import 'react-flot/flot/jquery.flot.crosshair.min';
import './TimelineChartPlugin';

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
      this.props.onSelect(
        Math.round(ranges.xaxis.from / 1000).toString(),
        Math.round(ranges.xaxis.to / 1000).toString()
      );
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
      height: this.props.height || '400px',
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
