/* eslint-disable 
@typescript-eslint/no-unsafe-member-access, 
@typescript-eslint/no-unsafe-call, 
func-names, 
@typescript-eslint/no-unsafe-return, 
@typescript-eslint/no-unsafe-assignment,
no-undef,
prefer-destructuring
*/
import React from 'react';
import * as ReactDOM from 'react-dom';
import type { ExploreTooltipProps } from '@webapp/components/TimelineChart/ExploreTooltip';
import { PlotType, EventHolderType, EventType } from './types';
import getFormatLabel from './getFormatLabel';
import clamp from './clamp';
import injectTooltip from './injectTooltip';

type ContextType = {
  init: (plot: PlotType) => void;
  options: ShamefulAny;
  name: string;
  version: string;
  onHoverDisplayTooltip?: (
    data: ExploreTooltipProps
  ) => React.FC<ExploreTooltipProps>;
};

const TOOLTIP_WRAPPER_ID = 'explore_tooltip_parent';

(function ($: JQueryStatic) {
  //  function init(this: ContextType, plot: PlotType) {
  //function init(plot: plotInitPluginParams) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const exploreTooltip = injectTooltip($, TOOLTIP_WRAPPER_ID);

    const params = {
      canvasX: -1,
      canvasY: -1,
      pageX: -1,
      pageY: -1,
      xToTime: -1,
    };

    function onMouseMove(e: EventType) {
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
      const options = plot.getOptions();
      // TODO(eh-am): fix type
      const onHoverDisplayTooltip = (options as any).onHoverDisplayTooltip;
      const { xaxis } = plot.getAxes() as ShamefulAny;
      const data = plot.getData();

      if (onHoverDisplayTooltip && exploreTooltip?.length) {
        const align = params.canvasX > plot.width() / 2 ? 'left' : 'right';
        const timezone = options.xaxis!.timezone;

        const timeLabel = getFormatLabel({
          date: params.xToTime,
          xaxis,
          timezone,
        });

        const values = data?.map((dataSeries, i) => {
          // Sometimes we also pass a tagName/color
          // Eg in tagExplorer page
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

        const Tooltip: React.ReactElement<
          ExploreTooltipProps,
          string | React.JSXElementConstructor<ExploreTooltipProps>
        >[] = onHoverDisplayTooltip({
          pageX: params.pageX,
          pageY: params.pageY,
          timeLabel,
          values,
          align,
          canvasX: params.canvasX,

          coordsToCanvasPos: plot.p2c.bind(plot),
        });

        ReactDOM.render(Tooltip, exploreTooltip?.[0]);
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
