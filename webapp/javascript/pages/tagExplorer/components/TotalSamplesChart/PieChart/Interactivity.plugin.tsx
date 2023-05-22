import React from 'react';
import ReactDOM from 'react-dom';
import injectTooltip from '@webapp/components/TimelineChart/injectTooltip';
import { ITooltipWrapperProps } from '@webapp/components/TimelineChart/TooltipWrapper';

const TOOLTIP_WRAPPER_ID = 'explore_tooltip_parent';

type ObjType = {
  series: {
    label: string;
    percent: number;
  };
  datapoint: number[][][];
};

type PositionType = {
  pageX: number;
  pageY: number;
};

(function ($: JQueryStatic) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const tooltipWrapper = injectTooltip($, TOOLTIP_WRAPPER_ID);

    function onPlotHover(_: unknown, pos: PositionType, obj: ObjType) {
      const options = plot.getOptions() as jquery.flot.plotOptions & {
        pieChartTooltip: (
          props: Omit<
            ITooltipWrapperProps & {
              label?: string;
              percent?: number;
              value?: number;
            },
            'children'
          >
        ) => React.ReactElement;
      };
      const tooltip = options?.pieChartTooltip;

      if (tooltip && tooltipWrapper?.length) {
        const value = obj?.datapoint?.[1]?.[0]?.[1];

        $('#total-samples-chart canvas.flot-overlay').css(
          'cursor',
          value ? 'crosshair' : 'default'
        );

        const Tooltip = tooltip({
          pageX: value ? pos.pageX : -1,
          pageY: value ? pos.pageY : -1,
          align: 'right',
          label: obj?.series?.label,
          percent: obj?.series.percent,
          value,
        });

        ReactDOM.render(<>{Tooltip}</>, tooltipWrapper?.[0]);
      }
    }

    plot.hooks!.bindEvents!.push(() => {
      plot.getPlaceholder().bind('plothover', onPlotHover);
    });

    plot.hooks!.shutdown!.push(() => {
      plot.getPlaceholder().unbind('plothover', onPlotHover);
    });
  }

  $.plot.plugins.push({
    init,
    options: {},
    name: 'interactivity',
    version: '1.0',
  });
})(jQuery);
