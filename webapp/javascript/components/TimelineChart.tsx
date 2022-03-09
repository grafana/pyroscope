import { connect } from 'react-redux';
import 'react-dom';
import React from 'react';

import ReactFlot from 'react-flot';
import 'react-flot/flot/jquery.flot.time.min';
import 'react-flot/flot/jquery.flot.selection.min';
import 'react-flot/flot/jquery.flot.crosshair.min';
import './TimelineChartPlugin';

import { bindActionCreators } from 'redux';
import {
  setDateRange,
  setLeftDateRange,
  setRightDateRange,
} from '../redux/actions';

interface TimelineChartProps {
  onSelect: (from: string, until: string) => void;
  className: string;
  viewSide: string;
}

class TimelineChart extends ReactFlot<TimelineChartProps> {
  componentDidMount() {
    this.draw();

    $(`#${this.props.id}`).bind('plotselected', (event, ranges) => {
      this.props.onSelect(
        Math.round(ranges.xaxis.from / 1000).toString(),
        Math.round(ranges.xaxis.to / 1000).toString()
      );

      //      if (this.props.viewSide === 'both' || this.props.viewSide === 'none') {
      //        this.props.actions.setDateRange(
      //          Math.round(ranges.xaxis.from / 1000),
      //          Math.round(ranges.xaxis.to / 1000)
      //        );
      //      } else if (this.props.viewSide === 'left') {
      //        this.props.actions.setLeftDateRange(
      //          Math.round(ranges.xaxis.from / 1000),
      //          Math.round(ranges.xaxis.to / 1000)
      //        );
      //      } else if (this.props.viewSide === 'right') {
      //        this.props.actions.setRightDateRange(
      //          Math.round(ranges.xaxis.from / 1000),
      //          Math.round(ranges.xaxis.to / 1000)
      //        );
      //      }
    });
  }

  componentDidUpdate() {
    this.draw();
  }

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

const mapStateToProps = (state) => ({
  ...state.root,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      setDateRange,
      setLeftDateRange,
      setRightDateRange,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(TimelineChart);
