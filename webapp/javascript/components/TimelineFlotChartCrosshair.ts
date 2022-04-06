// extracted from https://github.com/rodrigowirth/react-flot/blob/master/flot/jquery.flot.crosshair.js
/* eslint-disable 
    no-var, 
    vars-on-top,
    @typescript-eslint/ban-ts-comment, 
    @typescript-eslint/no-unused-vars, 
    @typescript-eslint/no-explicit-any, 
    @typescript-eslint/no-shadow,
    func-names,
    no-param-reassign */

(function ($) {
  var options = {
    crosshair: {
      mode: null, // one of null, "x", "y" or "xy",
      color: 'rgba(170, 0, 0, 0.80)',
      lineWidth: 1,
    },
  };

  function init(plot: any) {
    // position of crosshair in pixels
    var crosshair = { x: -1, y: -1, locked: false };

    plot.setCrosshair = function setCrosshair(pos: any) {
      if (!pos) crosshair.x = -1;
      else {
        var o = plot.p2c(pos);
        crosshair.x = Math.max(0, Math.min(o.left, plot.width()));
        crosshair.y = Math.max(0, Math.min(o.top, plot.height()));
      }

      plot.triggerRedrawOverlay();
    };

    plot.clearCrosshair = plot.setCrosshair; // passes null for pos

    plot.lockCrosshair = function lockCrosshair(pos: any) {
      if (pos) plot.setCrosshair(pos);
      crosshair.locked = true;
    };

    plot.unlockCrosshair = function unlockCrosshair() {
      crosshair.locked = false;
    };

    function onMouseOut(e: any) {
      if (crosshair.locked) return;

      if (crosshair.x !== -1) {
        crosshair.x = -1;
        plot.triggerRedrawOverlay();
      }
    }

    function onMouseMove(e: any) {
      if (crosshair.locked) return;

      if (plot.getSelection && plot.getSelection()) {
        crosshair.x = -1; // hide the crosshair while selecting
        return;
      }

      var offset = plot.offset();
      crosshair.x = Math.max(0, Math.min(e.pageX - offset.left, plot.width()));
      crosshair.y = Math.max(0, Math.min(e.pageY - offset.top, plot.height()));
      plot.triggerRedrawOverlay();
    }

    plot.hooks.bindEvents.push(function (plot: any, eventHolder: any) {
      const opts = plot.getOptions();
      // if selection disabled we don't bind any events for the canvas and don't show the crosshair
      if (!opts.crosshair.mode || opts.selection.disabled) return;

      eventHolder.mouseout(onMouseOut);
      eventHolder.mousemove(onMouseMove);
    });

    plot.hooks.drawOverlay.push(function (plot: any, ctx: any) {
      var c = plot.getOptions().crosshair;
      if (!c.mode) return;

      var plotOffset = plot.getPlotOffset();

      ctx.save();
      ctx.translate(plotOffset.left, plotOffset.top);

      if (crosshair.x !== -1) {
        var adj = plot.getOptions().crosshair.lineWidth % 2 ? 0.5 : 0;

        ctx.strokeStyle = c.color;
        ctx.lineWidth = c.lineWidth;
        ctx.lineJoin = 'round';

        ctx.beginPath();
        if (c.mode.indexOf('x') !== -1) {
          var drawX = Math.floor(crosshair.x) + adj;
          ctx.moveTo(drawX, 0);
          ctx.lineTo(drawX, plot.height());
        }
        if (c.mode.indexOf('y') !== -1) {
          var drawY = Math.floor(crosshair.y) + adj;
          ctx.moveTo(0, drawY);
          ctx.lineTo(plot.width(), drawY);
        }
        ctx.stroke();
      }
      ctx.restore();
    });

    plot.hooks.shutdown.push(function (plot: any, eventHolder: any) {
      eventHolder.unbind('mouseout', onMouseOut);
      eventHolder.unbind('mousemove', onMouseMove);
    });
  }

  $.plot.plugins.push({
    init,
    options,
    name: 'crosshair',
    version: '1.0',
  });
  // @ts-ignore
})(jQuery);
