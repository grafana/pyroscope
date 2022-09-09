import { PlotType } from '@webapp/components/TimelineChart/types';

(function ($: JQueryStatic) {
  function init(plot: PlotType) {
    function onPlotClick(
      event: ShamefulAny,
      pos: ShamefulAny,
      obj: ShamefulAny
    ) {
      console.log('obj', obj);
    }

    plot.hooks.bindEvents.push(() => {
      plot.getPlaceholder().bind('plotclick', onPlotClick);
    });

    plot.hooks.shutdown.push(() => {
      plot.getPlaceholder().unbind('plotclick', onPlotClick);
    });
  }

  ($ as ShamefulAny).plot.plugins.push({
    init,
    options: {},
    name: 'interactivity',
    version: '1.0',
  });
})(jQuery);
