/* eslint-disable */
// ISC License

// Copyright (c) 2018, Mapbox

// Permission to use, copy, modify, and/or distribute this software for any purpose
// with or without fee is hereby granted, provided that the above copyright notice
// and this permission notice appear in all copies.

// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
// REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
// FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
// INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
// OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
// TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
// THIS SOFTWARE.

// This component is based on flamebearer project
//   https://github.com/mapbox/flamebearer

import React from "react";
import { connect } from "react-redux";
import { bindActionCreators } from "redux";
import TimelineChart from "./TimelineChart";
import { formatAsOBject } from "../util/formatDate";



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
      }
    }}
  }

  componentDidMount() {
    let newFlotOptions = this.state.flotOptions;
    newFlotOptions.grid.markings = this.plotMarkings();

    this.setState({flotOptions: newFlotOptions})
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


  render = () => {
    const flotData = this.props.timeline
    ? [this.props.timeline.map((x) => [x[0], x[1] === 0 ? null : x[1] - 1])]
    : [];
    
    return (
      <TimelineChart
        id={this.props.id}
        options={this.state.flotOptions}
        data={flotData}
        width="100%"
        height="100px"
      />
    )
  }
}

const mapStateToProps = (state) => ({
  ...state,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    { },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(TimelineChartWrapper);
