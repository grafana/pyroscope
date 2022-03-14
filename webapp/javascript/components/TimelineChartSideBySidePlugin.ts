// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck
/* eslint-disable */
// plugin from https://github.com/pkoltermann/SideBySideImproved

(function ($) {
  function init(plot) {
    var orderedBarSeries;
    var nbOfBarsToOrder;
    var seriesPos = new Array();
    var sameSeries = new Array();
    var borderWidth;
    var borderWidthInXabsWidth;
    var pixelInXWidthEquivalent = 1;
    var isHorizontal = false;

    /*
     * This method add shift to x values
     */
    function reOrderBars(plot, serie, datapoints) {
      var shiftedPoints = null;

      // Added by eh-am
      //debugger;

      if (serieNeedToBeReordered(serie)) {
        checkIfGraphIsHorizontal(serie);
        calculPixel2XWidthConvert(plot);
        retrieveBarSeries(plot);
        calculBorderAndBarWidth(serie);

        if (nbOfBarsToOrder >= 2) {
          var position = findPosition(serie);
          var decallage = 0;

          var centerBarShift = calculCenterBarShift();

          if (isBarAtLeftOfCenter(position)) {
            decallage =
              -1 *
                sumWidth(
                  orderedBarSeries,
                  position - 1,
                  Math.floor(nbOfBarsToOrder / 2) - 1
                ) -
              centerBarShift;
          } else {
            decallage =
              sumWidth(
                orderedBarSeries,
                Math.ceil(nbOfBarsToOrder / 2),
                position - 2
              ) +
              centerBarShift +
              borderWidthInXabsWidth * 2;
          }

          shiftedPoints = shiftPoints(datapoints, serie, decallage);
          datapoints.points = shiftedPoints;
        } else if (nbOfBarsToOrder == 1) {
          // To be consistent with the barshift at other uneven numbers of bars, where
          // the center bar is centered around the point, we also need to shift a single bar
          // left by half its width
          var centerBarShift = -1 * calculCenterBarShift();
          shiftedPoints = shiftPoints(datapoints, serie, centerBarShift);
          datapoints.points = shiftedPoints;
        }
      }
      return shiftedPoints;
    }

    function serieNeedToBeReordered(serie) {
      return serie.bars != null && serie.bars.show && serie.bars.order != null;
    }

    function calculPixel2XWidthConvert(plot) {
      var gridDimSize = isHorizontal
        ? plot.getPlaceholder().innerHeight()
        : plot.getPlaceholder().innerWidth();
      var minMaxValues = isHorizontal
        ? getAxeMinMaxValues(plot.getData(), 1)
        : getAxeMinMaxValues(plot.getData(), 0);
      var AxeSize = minMaxValues[1] - minMaxValues[0];
      pixelInXWidthEquivalent = AxeSize / gridDimSize;
    }

    function getAxeMinMaxValues(series, AxeIdx) {
      var minMaxValues = new Array();
      for (var i = 0; i < series.length; i++) {
        minMaxValues[0] = series[i].data[0][AxeIdx];
        minMaxValues[1] = series[i].data[series[i].data.length - 1][AxeIdx];
      }
      if (typeof minMaxValues[0] == 'string') {
        minMaxValues[0] = 0;
        minMaxValues[1] = series[0].data.length - 1;
      }
      return minMaxValues;
    }

    function retrieveBarSeries(plot) {
      orderedBarSeries = findOthersBarsToReOrders(plot.getData());
      nbOfBarsToOrder = orderedBarSeries.length;
    }

    function findOthersBarsToReOrders(series) {
      var retSeries = new Array();

      for (var i = 0; i < series.length; i++) {
        if (series[i].bars.order != null && series[i].bars.show) {
          retSeries.push(series[i]);
        }
      }

      return sortByOrder(retSeries);
    }

    function sortByOrder(series) {
      var n = series.length;
      do {
        for (var i = 0; i < n - 1; i++) {
          if (series[i].bars.order > series[i + 1].bars.order) {
            var tmp = series[i];
            series[i] = series[i + 1];
            series[i + 1] = tmp;
          } else if (series[i].bars.order == series[i + 1].bars.order) {
            //check if any of the series has set sameSeriesArrayIndex
            var sameSeriesIndex;
            if (series[i].sameSeriesArrayIndex) {
              if (series[i + 1].sameSeriesArrayIndex !== undefined) {
                sameSeriesIndex = series[i].sameSeriesArrayIndex;
                series[i + 1].sameSeriesArrayIndex = sameSeriesIndex;
                sameSeries[sameSeriesIndex].push(series[i + 1]);
                sameSeries[sameSeriesIndex].sort(sortByWidth);

                series[i] = sameSeries[sameSeriesIndex][0];
                removeElement(series, i + 1);
              }
            } else if (series[i + 1].sameSeriesArrayIndex) {
              if (series[i].sameSeriesArrayIndex !== undefined) {
                sameSeriesIndex = series[i + 1].sameSeriesArrayIndex;
                series[i].sameSeriesArrayIndex = sameSeriesIndex;
                sameSeries[sameSeriesIndex].push(series[i]);
                sameSeries[sameSeriesIndex].sort(sortByWidth);

                series[i] = sameSeries[sameSeriesIndex][0];
                removeElement(series, i + 1);
              }
            } else {
              sameSeriesIndex = sameSeries.length;
              sameSeries[sameSeriesIndex] = new Array();
              series[i].sameSeriesArrayIndex = sameSeriesIndex;
              series[i + 1].sameSeriesArrayIndex = sameSeriesIndex;
              sameSeries[sameSeriesIndex].push(series[i]);
              sameSeries[sameSeriesIndex].push(series[i + 1]);
              sameSeries[sameSeriesIndex].sort(sortByWidth);

              series[i] = sameSeries[sameSeriesIndex][0];
              removeElement(series, i + 1);
            }
            i--;
            n--;

            //leave the wider serie and the other one move to
          }
        }
        n = n - 1;
      } while (n > 1);
      for (var i = 0; i < series.length; i++) {
        if (series[i].sameSeriesArrayIndex) {
          seriesPos[series[i].sameSeriesArrayIndex] = i;
        }
      }
      return series;
    }

    function sortByWidth(serie1, serie2) {
      var x = serie1.bars.barWidth ? serie1.bars.barWidth : 1;
      var y = serie2.bars.barWidth ? serie2.bars.barWidth : 1;
      return x < y ? -1 : x > y ? 1 : 0;
    }

    function removeElement(arr, from, to) {
      var rest = arr.slice((to || from) + 1 || arr.length);
      arr.length = from < 0 ? arr.length + from : from;
      arr.push.apply(arr, rest);
      return arr;
    }

    function calculBorderAndBarWidth(serie) {
      borderWidth =
        typeof serie.bars.lineWidth === 'number' ? serie.bars.lineWidth : 2;
      borderWidthInXabsWidth = borderWidth * pixelInXWidthEquivalent;
    }

    function checkIfGraphIsHorizontal(serie) {
      if (serie.bars.horizontal) {
        isHorizontal = true;
      }
    }

    function findPosition(serie) {
      var ss = sameSeries;
      var pos = 0;
      if (serie.sameSeriesArrayIndex) {
        pos = seriesPos[serie.sameSeriesArrayIndex];
      } else {
        for (var i = 0; i < orderedBarSeries.length; ++i) {
          if (serie == orderedBarSeries[i]) {
            pos = i;
            break;
          }
        }
      }
      return pos + 1;
    }

    function calculCenterBarShift() {
      var width = 0;

      if (nbOfBarsToOrder % 2 != 0)
        // Since the array indexing starts at 0, we need to use Math.floor instead of
        // Math.ceil otherwise we will get an error if there is only one bar
        width =
          orderedBarSeries[Math.floor(nbOfBarsToOrder / 2)].bars.barWidth / 2;

      return width;
    }

    function isBarAtLeftOfCenter(position) {
      return position <= Math.ceil(nbOfBarsToOrder / 2);
    }

    function sumWidth(series, start, end) {
      var totalWidth = 0;

      for (var i = start; i <= end; i++) {
        totalWidth += series[i].bars.barWidth + borderWidthInXabsWidth * 2;
      }

      return totalWidth;
    }

    function shiftPoints(datapoints, serie, dx) {
      var ps = datapoints.pointsize;
      var points = datapoints.points;
      var j = 0;
      for (var i = isHorizontal ? 1 : 0; i < points.length; i += ps) {
        points[i] += dx;
        //Adding the new x value in the serie to be abble to display the right tooltip value,
        //using the index 3 to not overide the third index.
        serie.data[j][3] = points[i];
        j++;
      }

      return points;
    }

    plot.hooks.processDatapoints.push(reOrderBars);
  }

  var options = {
    series: {
      bars: {
        order: null,
      }, // or number/string
    },
  };

  $.plot.plugins.push({
    init: init,
    options: options,
    name: 'orderBars',
    version: '0.2',
  });
})(jQuery);
