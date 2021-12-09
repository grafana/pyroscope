/* eslint-disable */

import React from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import TimelineChart from './TimelineChart';
import { formatAsOBject } from '../util/formatDate';
class TimelineChartWrapper extends React.Component {
  constructor() {
    super();

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
          margin: {
            left: 16,
            right: 16,
          },
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
  }

  componentDidMount() {
    let newFlotOptions = this.state.flotOptions;
    newFlotOptions.grid.markings = this.plotMarkings();

    this.setState({ flotOptions: newFlotOptions });
  }

  componentDidUpdate(prevProps) {
    if (this.props.viewSide == 'none') return;

    if (
      prevProps.leftFrom !== this.props.leftFrom ||
      prevProps.leftUntil !== this.props.leftUntil ||
      prevProps.rightFrom !== this.props.rightFrom ||
      prevProps.rightUntil !== this.props.rightUntil
    ) {
      let newFlotOptions = this.state.flotOptions;
      newFlotOptions.grid.markings = this.plotMarkings();

      this.setState({ flotOptions: newFlotOptions });
    }
  }

  plotMarkings = () => {
    if (!this.props.viewSide) {
      return null;
    }

    let leftFromInt = new Date(formatAsOBject(this.props.leftFrom)).getTime();
    let leftUntilInt = new Date(formatAsOBject(this.props.leftUntil)).getTime();
    let rightFromInt = new Date(formatAsOBject(this.props.rightFrom)).getTime();
    let rightUntilInt = new Date(
      formatAsOBject(this.props.rightUntil)
    ).getTime();

    let nonActiveBorder = 0.2;
    let nonActiveBackground = 0.09;

    let leftMarkings = [
      {
        xaxis: {
          from: leftFromInt,
          to: leftUntilInt,
        },
        color:
          this.props.viewSide === 'left'
            ? 'rgba(200, 102, 204, 0.35)'
            : `rgba(255, 102, 204, ${nonActiveBackground})`,
      },
      {
        color:
          this.props.viewSide === 'left'
            ? 'rgba(200, 102, 204, 1)'
            : `rgba(255, 102, 204, ${nonActiveBorder})`,
        lineWidth: 3,
        xaxis: { from: leftFromInt, to: leftFromInt },
      },
      {
        color:
          this.props.viewSide === 'left'
            ? 'rgba(200, 102, 204, 1)'
            : `rgba(255, 102, 204, ${nonActiveBorder})`,
        lineWidth: 3,
        xaxis: { from: leftUntilInt, to: leftUntilInt },
      },
    ];

    let rightMarkings = [
      {
        xaxis: {
          from: rightFromInt,
          to: rightUntilInt,
        },
        color:
          this.props.viewSide === 'right'
            ? 'rgba(19, 152, 246, 0.35)'
            : `rgba(19, 152, 246, ${nonActiveBackground})`,
      },
      {
        color:
          this.props.viewSide === 'right'
            ? 'rgba(19, 152, 246, 1)'
            : `rgba(19, 152, 246, ${nonActiveBorder})`,
        lineWidth: 3,
        xaxis: { from: rightFromInt, to: rightFromInt },
      },
      {
        color:
          this.props.viewSide === 'right'
            ? 'rgba(19, 152, 246, 1)'
            : `rgba(19, 152, 246, ${nonActiveBorder})`,
        lineWidth: 3,
        xaxis: { from: rightUntilInt, to: rightUntilInt },
      },
    ];

    return this.props.viewSide === 'none'
      ? []
      : leftMarkings.concat(rightMarkings);
  };

  render = () => {
    const flotData = this.props.timeline
      ? [this.props.timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
      : [];

    return (
      <TimelineChart
        id={this.props.id}
        options={this.state.flotOptions}
        viewSide={this.props.viewSide}
        data={flotData}
        width="100%"
        height="100px"
      />
    );
  };
}

const mapStateToProps = (state) => ({
  ...state.root,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({}, dispatch),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(TimelineChartWrapper);
