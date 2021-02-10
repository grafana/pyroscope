import React from "react";
import { connect } from "react-redux";

import { setDateRange } from "../redux/actions";
import Chart from 'react-apexcharts'

function TimelineChartApex(props) {
  const { from, until, setDateRange } = props;

  
  let series = [
    {
      name: "CPU Load",
      data: props.data
    }
  ]

  let options = {
    chart: {
      height: 380,
      width: "100%",
      type: "line"
    },
    series: series,
    xaxis: {
      type: 'datetime',
      datetimeFormatter: {
        year: 'yyyy',
        month: "MM",
        day: 'dd',
        hour: 'HH:mm',
    },
    },
    yaxis: {
      labels: {
        show: false,
      },
      axisBorder: {
        show: false,
        color: '#ffffff',
        offsetX: 0,
        offsetY: 0
      },
    },
    grid: {
      show: false,
      borderColor: '#90A4AE',
      strokeDashArray: 0,
      position: 'back',
      xaxis: {
          lines: {
              show: false
          }
      },   
      yaxis: {
          lines: {
              show: true
          }
      },  
      row: {
          colors: undefined,
          opacity: 0.5
      },  
      column: {
          colors: undefined,
          opacity: 0.5
      },  
      padding: {
          top: 0,
          right: 0,
          bottom: 0,
          left: 0
      },  
  },
    labels: {
      show: false,
    },
    stroke: {
      show: true,
      curve: 'straight',
      lineCap: 'butt',
      colors: '#E8BE3F',
      width: 1,
      dashArray: 0,      
    }
  }
  
  return (
    <Chart id="timeline-chart" options={options} series={series} type="line" height={150} />
  );
}

export default connect((x) => x, { setDateRange })(TimelineChartApex);
