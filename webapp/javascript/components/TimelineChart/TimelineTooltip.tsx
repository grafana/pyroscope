/* eslint-disable @typescript-eslint/no-unsafe-member-access, @typescript-eslint/no-unsafe-call */
import { PlotType, CtxType, EventHolderType, EventType } from './types';
import { clamp } from './utils';

(function ($: JQueryStatic) {
  function init(plot: PlotType) {
    let params = { mouseX: -1, mouseY: -1 };

    function onMouseMove(e: EventType) {
      //   console.log('e', e);
      const offset = plot.getPlaceholder().offset();
      const plotOffset = plot.getPlotOffset();

      const x = clamp(0, e.pageX - offset.left - plotOffset.left, plot.width());
      const y = clamp(0, e.pageY - offset.top - plotOffset.top, plot.height());

      params.mouseX = x;
      params.mouseY = y;

      //   console.log({ x, y });
    }

    function onMouseLeave() {
      params.mouseX = -1;
      params.mouseY = -1;
    }

    plot.hooks.drawOverlay.push((p: PlotType, ctx: CtxType) => {
      console.log('!', params);
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
