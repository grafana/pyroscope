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
import { PlotType, EventHolderType, EventType } from './types';
import { clamp, getFormatLabel } from './utils';

type ContextType = {
  init: (plot: PlotType) => void;
  options: ShamefulAny;
  name: string;
  version: string;
  exploreTooltip: ShamefulAny;
};

const TOOLTIP_WRAPPER_ID = 'explore_tooltip_parent';

(function ($: JQueryStatic) {
  function init(this: ContextType, plot: PlotType) {
    const exploreTooltip = injectTooltip($);
    const selection = { active: false, from: -1, to: -1 };

    const params = {
      canvasX: -1,
      canvasY: -1,
      pageX: -1,
      pageY: -1,
      xToTime: -1,
    };

    function onMouseMove(e: EventType) {
      const offset = plot.getPlaceholder().offset();
      const plotOffset = plot.getPlotOffset();

      params.canvasX = clamp(
        0,
        e.pageX - offset.left - plotOffset.left,
        plot.width()
      );
      params.canvasY = clamp(
        0,
        e.pageY - offset.top - plotOffset.top,
        plot.height()
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

    function onPlotHover(e: EventType, position: { x: number }) {
      params.xToTime = position.x;
    }

    plot.hooks.drawOverlay.push(() => {
      const options = plot.getOptions();
      const Tooltip = options?.exploreTooltip;
      const { xaxis } = plot.getAxes() as ShamefulAny;
      const data = plot.getData();

      if (Tooltip && exploreTooltip?.length) {
        const align = params.canvasX > plot.width() / 2 ? 'left' : 'right';
        const timezone = options.xaxis.timezone;
        const timeLabel = selection.active
          ? `${getFormatLabel({
              date: selection.from,
              xaxis,
              timezone,
            })} - ${getFormatLabel({
              date: selection.to,
              xaxis,
              timezone,
            })}`
          : getFormatLabel({
              date: params.xToTime,
              xaxis,
              timezone,
            });

        const values = data?.map(
          (
            d: {
              data: number[][];
              tagName: string;
              color: { color: number[] };
            },
            i
          ) => {
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
          }
        );

        ReactDOM.render(
          <Tooltip
            pageX={params.pageX}
            pageY={params.pageY}
            align={align}
            timeLabel={timeLabel}
            values={values}
          />,
          exploreTooltip?.[0]
        );
      }
    });

    const onMouseUp = () => {
      selection.active = false;
    };

    const onMouseDown = () => {
      selection.active = true;
    };

    const onPlotSelecting = (
      e: EventType,
      selectionData: { xaxis: { from: number; to: number } }
    ) => {
      selection.from = selectionData?.xaxis?.from || -1;
      selection.to = selectionData?.xaxis?.to || -1;
    };

    plot.hooks.bindEvents.push((p: PlotType, eventHolder: EventHolderType) => {
      eventHolder.mousemove(onMouseMove);
      eventHolder.mouseleave(onMouseLeave);
      eventHolder.mouseup(onMouseUp);
      eventHolder.mousedown(onMouseDown);
      plot.getPlaceholder().bind('plothover', onPlotHover);
      // detect plotselecting event from ./TimelineChartSelection.ts
      plot.getPlaceholder().bind('plotselecting', onPlotSelecting);
    });

    plot.hooks.shutdown.push((_: PlotType, eventHolder: EventHolderType) => {
      eventHolder.unbind('mousemove', onMouseMove);
      eventHolder.unbind('mouseleave', onMouseLeave);
      eventHolder.unbind('mouseup', onMouseUp);
      eventHolder.unbind('mousedown', onMouseDown);
      plot.getPlaceholder().unbind('plothover', onPlotHover);
      plot.getPlaceholder().unbind('plotselecting', onPlotSelecting);
    });
  }

  ($ as ShamefulAny).plot.plugins.push({
    init,
    options: {},
    name: 'rich_tooltip',
    version: '1.0',
  });
})(jQuery);

const injectTooltip = ($: JQueryStatic) => {
  const tooltipParent = $(`#${TOOLTIP_WRAPPER_ID}`).length
    ? $(`#${TOOLTIP_WRAPPER_ID}`)
    : $(`<div id="${TOOLTIP_WRAPPER_ID}" />`);

  const par2 = $(`body`);

  return tooltipParent.appendTo(par2);
};
