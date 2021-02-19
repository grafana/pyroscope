import React from "react";
import { connect } from "react-redux";

import { setDateRange } from "../redux/actions";
import Chart from 'react-apexcharts'
import moment from "moment";
import { formatAsOBject } from "../util/formatDate";


function ComparisonTimeline(props) {
  const { setDateRange, timelineData, leftFrom, leftUntil, rightFrom, rightUntil } = props;

  const dateFormat = "YYYY-MM-DD hh:mm A";

  let series = [
    {
      name: "CPU Load",
      data: timelineData
    }
  ]

  let options = {
    stroke: {
      color: '#ff0000',
      opacity: 0.4,
      width: 1
    },
    annotations: {
      xaxis: [
        {
          x: new Date(formatAsOBject(leftFrom)).getTime(),
          x2: new Date(formatAsOBject(leftUntil)).getTime(),
          fillColor: '#AEA2E0',
          strokeDashArray: 0,
          borderColor: '#AEA2E0',
          label: {
            text: undefined
          }
        },
        {
          x: new Date(formatAsOBject(rightFrom)).getTime(),
          x2: new Date(formatAsOBject(rightUntil)).getTime(),
          fillColor: '#83B5D8',
          strokeDashArray: 0,
          borderColor: '#83B5D8',
          label: {
            text: undefined
          }
        }
      ]
    },
    chart: {
      offsetY: -10,
      height: 380,
      width: "100%",
      type: "line",
      animations: {
        enabled: false,
        easing: 'easeinout',
        speed: 800,
        animateGradually: {
            enabled: false,
            delay: 0
        },
        dynamicAnimation: {
            enabled: false,
            speed: 0
        }
      },
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
          setDateRange(
            Math.round(xaxis.min / 1000),
            Math.round(xaxis.max / 1000),
          )
          return
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
      axisBorder: {
        show: true,
        color: '#424446',
        height: 1,
        offsetX: 0,
        offsetY: 0
      },
      axisTicks: {

        show: true,
        borderType: 'solid',
        color: '#424446',
        height: 6,
        offsetX: 0,
        offsetY: 0
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
      borderColor: '#424446',
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
          formatter: function(val, opts) {
            return moment(val).format(dateFormat);
          }
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

export default connect((x) => x, { setDateRange })(ComparisonTimeline);
