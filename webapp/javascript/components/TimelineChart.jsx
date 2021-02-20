import { connect } from "react-redux";
import "react-dom";
import React from "react";

import ReactFlot from "react-flot";
import "react-flot/flot/jquery.flot.time.min";
import "react-flot/flot/jquery.flot.selection.min";
import "react-flot/flot/jquery.flot.crosshair.min";
import { bindActionCreators } from "redux";
import { setDateRange } from "../redux/actions";
import { formatAsOBject } from "../util/formatDate";

class TimelineChart extends ReactFlot {
  constructor() {
    super();
  }

  plotMarkings = () => {
    if (!this.props.showMarkings) {
      return null;
    }

    let leftFromInt = new Date(formatAsOBject(this.props.leftFrom)).getTime()
    let leftUntilInt = new Date(formatAsOBject(this.props.leftUntil)).getTime()
    let rightFromInt = new Date(formatAsOBject(this.props.rightFrom)).getTime()
    let rightUntilInt = new Date(formatAsOBject(this.props.rightUntil)).getTime()

    let leftMarkings = [
      {
        xaxis: {
          from: leftFromInt,
          to: leftUntilInt
        },
        yaxis: {
          from: 0,
          to: 1000
        },
        color: "rgba(235, 168, 230, 0.35)",
        opacity: 0.5,
      },         
      { 
        color: "rgba(235, 168, 230, 1)", 
        lineWidth: 2, 
        xaxis: { from: leftFromInt, to: leftFromInt } 
      },
      { 
        color: "rgba(235, 168, 230, 1)", 
        lineWidth: 2, 
        xaxis: { from: leftUntilInt, to: leftUntilInt } 
      },
    ]

    let rightMarkings = [
      { 
        xaxis: { 
          from: rightFromInt,
          to: rightUntilInt
        }, 
        yaxis: { 
          from: 0, 
          to: 1000 
        }, 
        color: "rgba(81,  149, 206, 0.35)" 
      },
      { 
        color: "rgba(81,  149, 206, 1)" , 
        lineWidth: 2, 
        xaxis: { from: rightFromInt, to: rightFromInt } 
      },
      { 
        color: "rgba(81,  149, 206, 1)" , 
        lineWidth: 2, 
        xaxis: { from: rightUntilInt, to: rightUntilInt } 
      },
    ]

    return {
      left: leftMarkings,
      right: rightMarkings,
      both: leftMarkings.concat(rightMarkings),
      none: []
    }[this.props.showMarkings];
  };

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
      <ReactFlot id={this.props.id} options={this.props.options} data={this.props.data || [[0, 0]]} width={this.props.width} height="100px" />
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
