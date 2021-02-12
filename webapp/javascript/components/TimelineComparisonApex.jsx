import React from "react";
import { connect } from "react-redux";

import { setLeftDateRange, setRightDateRange } from "../redux/actions";
import Chart from 'react-apexcharts'
// import ApexChart from 'react-apexcharts';
import moment from "moment";
import { formatAsOBject } from "../util/formatDate";


function TimelineComparisonApex(props) {
  const { leftFrom, leftUntil, rightFrom, rightUntil, timelineData, side, setLeftDateRange, setRightDateRange } = props;

  const dateFormat = "YYYY-MM-DD hh:mm A";

  let series = [
    {
      name: "CPU Load",
      data: timelineData
    }
  ]

  let annotation = side == 'left' ?
    {
      x: new Date(formatAsOBject(leftFrom)).getTime(),
      x2: new Date(formatAsOBject(leftUntil)).getTime(),
      fillColor: '#AEA2E0',
      label: {
        text: undefined
      }
    } :
    {
      x: new Date(formatAsOBject(rightFrom)).getTime(),
      x2: new Date(formatAsOBject(rightUntil)).getTime(),
      fillColor: '#83B5D8',
      label: {
        text: undefined
      }
    }

  let options = {
    annotations: {
      xaxis: [annotation]
    },
    chart: {
      height: 380,
      width: "100%",
      type: "line",
      animations: {
        enabled: false,
        easing: 'easeinout',
        speed: 800,
        animateGradually: {
            enabled: false,
            delay: 150
        },
        dynamicAnimation: {
            enabled: false,
            speed: 350
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
          if (side == "left") {
            setLeftDateRange(
              Math.round(xaxis.min / 1000),
              Math.round(xaxis.max / 1000)
            );
          } else if (side == "right") {
            setRightDateRange(
              Math.round(xaxis.min / 1000),
              Math.round(xaxis.max / 1000)
            );
          } else {
            console.error('should not be here....')
          }
          return {
            xaxis: {
              min: timelineData[0][0],
              max: timelineData[timelineData.length - 1][0]
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
      <Chart options={options} series={series} type="line" height={150} />
    </div>
  );
}

export default connect((x) => x, { setLeftDateRange, setRightDateRange })(TimelineComparisonApex);
