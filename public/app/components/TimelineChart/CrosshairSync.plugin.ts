const defaultOptions = {
  syncCrosshairsWith: [],
};

// Enhances the default Plot object with the API from the crosshair plugin
// https://github.com/flot/flot/blob/de34ce947d8ebfb2cac0b682a130ba079d8d654b/source/jquery.flot.crosshair.js#L74
type PlotWithCrosshairsSupport = jquery.flot.plot &
  jquery.flot.plotOptions & {
    setCrosshair(pos: { x: number; y: number }): void;
    clearCrosshair(): void;
  };

(function ($) {
  function init(plot: PlotWithCrosshairsSupport) {
    function getOptions() {
      return plot.getOptions() as jquery.flot.plotOptions &
        typeof defaultOptions;
    }

    function accessExternalInstance(id: string) {
      // Access another flotjs instance
      // https://github.com/flot/flot/blob/de34ce947d8ebfb2cac0b682a130ba079d8d654b/source/jquery.flot.js#L969
      const p: PlotWithCrosshairsSupport = $(`#${id}`).data('plot');
      return p;
    }

    function onPlotHover(
      syncCrosshairsWith: (typeof defaultOptions)['syncCrosshairsWith'],
      e: unknown,
      position: { x: number; y: number }
    ) {
      syncCrosshairsWith.forEach((id) =>
        accessExternalInstance(id).setCrosshair(position)
      );
    }

    function clearCrosshairs(
      syncCrosshairsWith: (typeof defaultOptions)['syncCrosshairsWith']
    ) {
      syncCrosshairsWith.forEach((id) =>
        accessExternalInstance(id).clearCrosshair()
      );
    }

    plot.hooks!.bindEvents!.push(() => {
      const options = getOptions();

      plot
        .getPlaceholder()
        .bind('plothover', onPlotHover.bind(null, options.syncCrosshairsWith));

      plot
        .getPlaceholder()
        .bind(
          'mouseleave',
          clearCrosshairs.bind(null, options.syncCrosshairsWith)
        );
    });

    plot.hooks!.shutdown!.push(() => {
      const options = getOptions();

      clearCrosshairs(options.syncCrosshairsWith);

      plot
        .getPlaceholder()
        .bind(
          'mouseleave',
          clearCrosshairs.bind(null, options.syncCrosshairsWith)
        );

      plot
        .getPlaceholder()
        .unbind(
          'plothover',
          onPlotHover.bind(null, options.syncCrosshairsWith)
        );
    });
  }

  $.plot.plugins.push({
    init,
    options: defaultOptions,
    name: 'crosshair-sync',
    version: '1.0',
  });
})(jQuery);

// TS1208: 'CrosshairSync.plugin.ts' cannot be compiled under '--isolatedModules' because it is considered a global script file.
// Add an import, export, or an empty 'export {}' statement to make it a module.
export {};
