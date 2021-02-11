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
      type: "line",
      toolbar: {
        show: false,
      },
      zoom: {
        enabled: true,
        zoomedArea: {
          fill: {
            color: '#90CAF9',
            opacity: 0.4
          },
          stroke: {
            color: '#0D47A1',
            opacity: 0.4,
            width: 1
          }
        }
      },
      events: {
        beforeZoom: function(chartContext, { xaxis }) {
          console.log('xaxis...', xaxis)
          return {
            xaxis: {
              min: xaxis.min,
              max: xaxis.max
            }
          } 
        },
        beforeResetZoom: undefined,
        zoomed: undefined,
        scrolled: undefined,
        scrolled: undefined,
      }
    },
    markers: {
      size: 0,
      colors: "#E8BE3F",
      strokeColors: '#E8BE3F',
      strokeWidth: 1,
      strokeOpacity: 0.9,
      strokeDashArray: 0,
      fillOpacity: 1,
      discrete: [],
      shape: "circle",
      radius: 2,
      offsetX: 0,
      offsetY: 0,
      onClick: undefined,
      onDblClick: undefined,
      showNullDataPoints: true,
      hover: {
        size: undefined,
        sizeOffset: 3
      }
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
    },
    tooltip: {
      enabled: true,
      enabledOnSeries: undefined,
      shared: false,
      followCursor: false,
      intersect: false,
      inverseOrder: false,
      custom: function({series, seriesIndex, dataPointIndex, w}) {
        return ''
      },
      fillSeriesColor: false,
      theme: false,
      style: {
        fontSize: '12px',
        fontFamily: undefined
      },
      onDatasetHover: {
        highlightDataSeries: false,
      },
      x: {
          show: true,
          format: 'yyyy-MM-dd hh:mm',
          formatter: undefined,
      },
      marker: {
        show: false,
      },
    }
  }
  
  return (
    <div className="timeline-chart-container">
      <Chart id="timeline-chart" options={options} series={series} type="line" height={150} />
    </div>
  );
}

export default connect((x) => x, { setDateRange })(TimelineChartApex);
