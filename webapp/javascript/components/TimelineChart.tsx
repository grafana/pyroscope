import React from 'react';
import { connect } from "react-redux";
import { setDateRange } from "../redux/actions";
import "react-dom";

import ReactFlot from 'react-flot';
import 'react-flot/flot/jquery.flot.time.min';
import 'react-flot/flot/jquery.flot.selection.min';


class TimelineChart extends ReactFlot {
  componentDidMount() {
    this.draw();
    $(`#${this.props.id}`).bind('plotselected', (event, ranges) => {
      this.props.setDateRange(Math.round(ranges.xaxis.from / 1000), Math.round(ranges.xaxis.to / 1000))
    });
  }
}

export default connect(
  (x) => x,
  {setDateRange}
)(TimelineChart);
