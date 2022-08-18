/* eslint-disable @typescript-eslint/no-unsafe-member-access, @typescript-eslint/no-unsafe-call, func-names */
import React from 'react';
import * as ReactDOM from 'react-dom';
import { PlotType, EventHolderType, EventType } from './types';
import { clamp } from './utils';

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
    this.exploreTooltip = null;

    const params = {
      canvasX: -1,
      canvasY: -1,
      pageX: -1,
      pageY: -1,
      parentSelector: null,
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

    plot.hooks.draw.push(() => {
      const o = plot.getOptions();

      const parentSelector = o.exploreTooltip?.parentSelector;

      if (parentSelector) {
        const tooltipParent = $(`#${TOOLTIP_WRAPPER_ID}`).length
          ? $(`#${TOOLTIP_WRAPPER_ID}`)
          : $(`<div id="${TOOLTIP_WRAPPER_ID}" />`);

        const par2 = $(parentSelector);

        this.exploreTooltip = tooltipParent.appendTo(par2);
      }
    });

    plot.hooks.drawOverlay.push(() => {
      const options = plot.getOptions();
      const canvasWidth = plot.width();

      const Tooltip = options?.exploreTooltip?.component;

      const align = params.canvasX > canvasWidth / 2 ? 'left' : 'right';

      if (Tooltip && this?.exploreTooltip?.length) {
        ReactDOM.render(
          <Tooltip pageX={params.pageX} pageY={params.pageY} align={align} />,
          this.exploreTooltip?.[0]
        );
      }
    });

    plot.hooks.bindEvents.push((p: PlotType, eventHolder: EventHolderType) => {
      eventHolder.mousemove(onMouseMove);
      eventHolder.mouseleave(onMouseLeave);
    });

    plot.hooks.shutdown.push((_: PlotType, eventHolder: EventHolderType) => {
      eventHolder.unbind('mousemove', onMouseMove);
      eventHolder.unbind('mouseleave', onMouseLeave);
    });
  }

  ($ as ShamefulAny).plot.plugins.push({
    init,
    options: {},
    name: 'rich_tooltip',
    version: '1.0',
  });
})(jQuery);
