import React from 'react';
import * as ReactDOM from 'react-dom';
import getFormatLabel from './getFormatLabel';
import clamp from './clamp';
import injectTooltip from './injectTooltip';
import { ITooltipWrapperProps } from './TooltipWrapper';

const TOOLTIP_WRAPPER_ID = 'explore_tooltip_parent';

// TooltipCallbackProps refers to the data available for the tooltip body construction
export interface TooltipCallbackProps {
  timeLabel: string;
  values: Array<{
    closest: number[];
    color: number[];
    // TODO: remove this
    tagName: string;
  }>;
  coordsToCanvasPos?: jquery.flot.axis['p2c'];
  canvasX?: number;
}

(function ($: JQueryStatic) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const exploreTooltip = injectTooltip($, TOOLTIP_WRAPPER_ID);

    const params = {
      canvasX: -1,
      canvasY: -1,
      pageX: -1,
      pageY: -1,
      xToTime: -1,
    };

    function onMouseMove(e: { pageX: number; pageY: number; which?: number }) {
      const offset = plot.getPlaceholder().offset()!;
      const plotOffset = plot.getPlotOffset();

      params.canvasX = clamp(
        0,
        plot.width(),
        e.pageX - offset.left - plotOffset.left
      );
      params.canvasY = clamp(
        0,
        plot.height(),
        e.pageY - offset.top - plotOffset.top
      );
      params.pageX = e.pageX;
      params.pageY = e.pageY;
    }

    function onMouseLeave() {
      params.canvasX = -1;
      params.canvasY = -1;
      params.pageX = -1;
      params.pageY = -1;
    }

    function onPlotHover(e: unknown, position: { x?: number }) {
      if (position.x) {
        params.xToTime = position.x;
      }
    }

    plot.hooks!.drawOverlay!.push(() => {
      const options = plot.getOptions() as jquery.flot.plotOptions & {
        onHoverDisplayTooltip?: (
          data: Omit<ITooltipWrapperProps & TooltipCallbackProps, 'children'>
        ) => React.ReactElement;
      };
      const { onHoverDisplayTooltip } = options;
      const { xaxis } = plot.getAxes() as ShamefulAny;
      const data = plot.getData();

      if (onHoverDisplayTooltip && exploreTooltip?.length) {
        const align = params.canvasX > plot.width() / 2 ? 'left' : 'right';
        const { timezone } = options.xaxis!;

        const timeLabel = getFormatLabel({
          date: params.xToTime,
          xaxis,
          timezone,
        });

        const values = data?.map((dataSeries, i) => {
          // Sometimes we also pass a tagName/color
          // Eg in tagExplorer page
          // TODO: use generics
          const d = dataSeries as jquery.flot.dataSeries & {
            tagName: string;
            color: { color: number[] };
          };

          let closest = null;
          let color = null;
          let tagName = String(i);

          if (d?.data?.length && params.xToTime && params.pageX > 0) {
            color = d?.color?.color;
            tagName = d.tagName;
            closest = (d?.data || []).reduce(function (prev, curr) {
              return Math.abs(curr?.[0] - params.xToTime) <
                Math.abs(prev?.[0] - params.xToTime)
                ? curr
                : prev;
            });
          }

          return {
            closest,
            color,
            tagName,
          };
        });

        if (!values?.length) {
          return;
        }

        // Returns an element
        const Tooltip = onHoverDisplayTooltip({
          pageX: params.pageX,
          pageY: params.pageY,
          timeLabel,
          values,
          align,
          canvasX: params.canvasX,

          coordsToCanvasPos: plot.p2c.bind(plot),
        });

        // Type checking will be wrong if a React 18 app tries to use this code
        ReactDOM.render(Tooltip as ShamefulAny, exploreTooltip?.[0]);
      }
    });

    plot.hooks!.bindEvents!.push((p, eventHolder) => {
      eventHolder.mousemove(onMouseMove);
      eventHolder.mouseleave(onMouseLeave);
      plot.getPlaceholder().bind('plothover', onPlotHover);
    });

    plot.hooks!.shutdown!.push((p, eventHolder) => {
      eventHolder.unbind('mousemove', onMouseMove);
      eventHolder.unbind('mouseleave', onMouseLeave);
      plot.getPlaceholder().unbind('plothover', onPlotHover);
    });
  }

  $.plot.plugins.push({
    init,
    options: {},
    name: 'rich_tooltip',
    version: '1.0',
  });
})(jQuery);
