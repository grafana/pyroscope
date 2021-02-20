import { connect } from "react-redux";
import "react-dom";
import React from "react";

import ReactFlot from "react-flot";
import "react-flot/flot/jquery.flot.time.min";
import "react-flot/flot/jquery.flot.selection.min";
import "react-flot/flot/jquery.flot.crosshair.min";
import { bindActionCreators } from "redux";
import { setDateRange } from "../redux/actions";

class TimelineChart extends ReactFlot {
  constructor() {
    super();

    this.flotOptions = {
      margin: {
        top: 0,
        left: 0,
        bottom: 0,
        right: 0,
      },
      selection: {
        mode: "x",
      },
      crosshair: {
        mode: "x",
        color: "#C3170D",
        lineWidth: "1",
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
        mode: "time",
        timezone: "browser",
        reserveSpace: false,
      },
    };
  }

  componentDidMount() {
    this.draw();
    $(`#${this.props.id}`).bind("plotselected", (event, ranges) => {
      this.props.actions.setDateRange(
        Math.round(ranges.xaxis.from / 1000),
        Math.round(ranges.xaxis.to / 1000)
      );
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

  render = () => {
    return (
      <ReactFlot id={this.props.id} options={this.flotOptions} data={this.props.data} width={this.props.width} height="100px" />
    )
  }
}

const mapStateToProps = (state) => ({
  ...state,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      setDateRange,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(TimelineChart);
