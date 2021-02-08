import React from "react";
import { connect } from "react-redux";
import "react-dom";

import ReactFlot from "react-flot";
import "react-flot/flot/jquery.flot.time.min";
import "react-flot/flot/jquery.flot.selection.min";
import "react-flot/flot/jquery.flot.crosshair.min";
import { bindActionCreators } from "redux";
import { setLeftDateRange, setRightDateRange, setDateRange, receiveJSON } from "../redux/actions";

class TimelineComparison extends ReactFlot {
  componentDidMount() {
    this.draw();
    $(`#${this.props.id}`).bind("plotselected", (event, ranges) => {
      console.log('ranges: ', ranges)
      console.log('event: ', event)
      console.log('ranges: ', ranges)
      console.log('event: ', event)

      if (this.props.side == "left") {
        this.props.actions.setLeftDateRange(
          Math.round(ranges.xaxis.from / 1000),
          Math.round(ranges.xaxis.to / 1000)
        );
      } else if (this.props.side == "right") {
        this.props.actions.setRightDateRange(
          Math.round(ranges.xaxis.from / 1000),
          Math.round(ranges.xaxis.to / 1000)
        );
      } else {
        console.error('should not be here....')
      }
    });

    $(`#${this.props.id}`).bind("plothover", (evt, position) => {
      if (position) {
        this.lockCrosshair({
          x: item.datapoint[0],
          y: item.datapoint[1],
        });
      } else {
        this.unlockCrosshair();
      }
    });
  }
}

const mapStateToProps = (state) => ({
  ...state,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      setDateRange,
      setLeftDateRange,
      setRightDateRange,
      receiveJSON,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(TimelineComparison);