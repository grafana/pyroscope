// extracted from https://github.com/rodrigowirth/react-flot/blob/master/flot/jquery.flot.selection.js
/* eslint-disable */

(function ($) {
  function init(plot: any) {
    var selection = {
      first: { x: -1, y: -1 },
      second: { x: -1, y: -1 },
      show: false,
      active: false,
    };

    // FIXME: The drag handling implemented here should be
    // abstracted out, there's some similar code from a library in
    // the navigation plugin, this should be massaged a bit to fit
    // the Flot cases here better and reused. Doing this would
    // make this plugin much slimmer.
    var savedhandlers: any = {};

    var mouseUpHandler: any = null;

    function onMouseMove(e: any) {
      if (selection.active) {
        updateSelection(e);

        plot.getPlaceholder().trigger('plotselecting', [getSelection()]);
      }
    }

    function onMouseDown(e: any) {
      if (e.which !== 1)
        // only accept left-click
        return;

      // cancel out any text selections
      document.body.focus();

      // prevent text selection and drag in old-school browsers
      if (
        document.onselectstart !== undefined &&
        savedhandlers.onselectstart == null
      ) {
        savedhandlers.onselectstart = document.onselectstart;
        document.onselectstart = function () {
          return false;
        };
      }
      if (document.ondrag !== undefined && savedhandlers.ondrag == null) {
        savedhandlers.ondrag = document.ondrag;
        document.ondrag = function () {
          return false;
        };
      }

      setSelectionPos(selection.first, e);

      selection.active = true;

      // this is a bit silly, but we have to use a closure to be
      // able to whack the same handler again
      mouseUpHandler = function (e: any) {
        onMouseUp(e);
      };

      $(document).one('mouseup', mouseUpHandler);
    }

    function onMouseUp(e: any) {
      mouseUpHandler = null;

      // revert drag stuff for old-school browsers
      if (document.onselectstart !== undefined)
        document.onselectstart = savedhandlers.onselectstart;
      if (document.ondrag !== undefined) document.ondrag = savedhandlers.ondrag;

      // no more dragging
      selection.active = false;
      updateSelection(e);

      if (selectionIsSane()) triggerSelectedEvent();
      else {
        // this counts as a clear
        plot.getPlaceholder().trigger('plotunselected', []);
        plot.getPlaceholder().trigger('plotselecting', [null]);
      }

      return false;
    }

    function getSelection() {
      if (!selectionIsSane()) return null;

      if (!selection.show) return null;

      var r: any = {},
        c1: any = selection.first,
        c2: any = selection.second;
      $.each(plot.getAxes(), function (name: any, axis: any) {
        if (axis.used) {
          var p1 = axis.c2p(c1[axis.direction]),
            p2 = axis.c2p(c2[axis.direction]);
          r[name] = { from: Math.min(p1, p2), to: Math.max(p1, p2) };
        }
      });
      return r;
    }

    function triggerSelectedEvent() {
      var r: any = getSelection();

      plot.getPlaceholder().trigger('plotselected', [r]);

      // backwards-compat stuff, to be removed in future
      if (r.xaxis && r.yaxis)
        plot.getPlaceholder().trigger('selected', [
          {
            x1: r.xaxis.from,
            y1: r.yaxis.from,
            x2: r.xaxis.to,
            y2: r.yaxis.to,
          },
        ]);
    }

    function clamp(min: any, value: any, max: any) {
      return value < min ? min : value > max ? max : value;
    }

    function setSelectionPos(pos: any, e: any) {
      var o = plot.getOptions();
      var offset = plot.getPlaceholder().offset();
      var plotOffset = plot.getPlotOffset();
      pos.x = clamp(0, e.pageX - offset.left - plotOffset.left, plot.width());
      pos.y = clamp(0, e.pageY - offset.top - plotOffset.top, plot.height());

      if (o.selection.mode == 'y')
        pos.x = pos == selection.first ? 0 : plot.width();

      if (o.selection.mode == 'x')
        pos.y = pos == selection.first ? 0 : plot.height();
    }

    function updateSelection(pos: any) {
      if (pos.pageX == null) return;

      setSelectionPos(selection.second, pos);
      if (selectionIsSane()) {
        selection.show = true;
        plot.triggerRedrawOverlay();
      } else clearSelection(true);
    }

    function clearSelection(preventEvent: any) {
      if (selection.show) {
        selection.show = false;
        plot.triggerRedrawOverlay();
        if (!preventEvent) plot.getPlaceholder().trigger('plotunselected', []);
      }
    }

    // function taken from markings support in Flot
    function extractRange(ranges: any, coord: any) {
      var axis,
        from,
        to,
        key,
        axes = plot.getAxes();

      for (var k in axes) {
        axis = axes[k];
        if (axis.direction == coord) {
          key = coord + axis.n + 'axis';
          if (!ranges[key] && axis.n == 1) key = coord + 'axis'; // support x1axis as xaxis
          if (ranges[key]) {
            from = ranges[key].from;
            to = ranges[key].to;
            break;
          }
        }
      }

      // backwards-compat stuff - to be removed in future
      if (!ranges[key as any]) {
        axis = coord == 'x' ? plot.getXAxes()[0] : plot.getYAxes()[0];
        from = ranges[coord + '1'];
        to = ranges[coord + '2'];
      }

      // auto-reverse as an added bonus
      if (from != null && to != null && from > to) {
        var tmp = from;
        from = to;
        to = tmp;
      }

      return { from: from, to: to, axis: axis };
    }

    function setSelection(ranges: any, preventEvent: any) {
      var axis,
        range,
        o = plot.getOptions();

      if (o.selection.mode == 'y') {
        selection.first.x = 0;
        selection.second.x = plot.width();
      } else {
        range = extractRange(ranges, 'x');

        selection.first.x = range.axis.p2c(range.from);
        selection.second.x = range.axis.p2c(range.to);
      }

      if (o.selection.mode == 'x') {
        selection.first.y = 0;
        selection.second.y = plot.height();
      } else {
        range = extractRange(ranges, 'y');

        selection.first.y = range.axis.p2c(range.from);
        selection.second.y = range.axis.p2c(range.to);
      }

      selection.show = true;
      plot.triggerRedrawOverlay();
      if (!preventEvent && selectionIsSane()) triggerSelectedEvent();
    }

    function selectionIsSane() {
      var minSize = plot.getOptions().selection.minSize;
      return (
        Math.abs(selection.second.x - selection.first.x) >= minSize &&
        Math.abs(selection.second.y - selection.first.y) >= minSize
      );
    }

    plot.clearSelection = clearSelection;
    plot.setSelection = setSelection;
    plot.getSelection = getSelection;

    plot.hooks.bindEvents.push(function (plot: any, eventHolder: any) {
      var o = plot.getOptions();
      // if selection disabled we don't bind any events for the canvas and don't allow user selecting
      if (o.selection.mode != null && !o.selection.disabled) {
        eventHolder.mousemove(onMouseMove);
        eventHolder.mousedown(onMouseDown);
      }
    });

    plot.hooks.drawOverlay.push(function (plot: any, ctx: any) {
      // draw selection
      if (selection.show && selectionIsSane()) {
        var plotOffset = plot.getPlotOffset();
        var o = plot.getOptions();

        ctx.save();
        ctx.translate(plotOffset.left, plotOffset.top);

        var c = $.color.parse(o.selection.color);

        ctx.strokeStyle = c.scale('a', 0.8).toString();
        ctx.lineWidth = 1;
        ctx.lineJoin = o.selection.shape;
        ctx.fillStyle = c.scale('a', 0.4).toString();

        var x = Math.min(selection.first.x, selection.second.x) + 0.5,
          y = Math.min(selection.first.y, selection.second.y) + 0.5,
          w = Math.abs(selection.second.x - selection.first.x) - 1,
          h = Math.abs(selection.second.y - selection.first.y) - 1;

        ctx.fillRect(x, y, w, h);
        ctx.strokeRect(x, y, w, h);

        ctx.restore();
      }
    });

    plot.hooks.shutdown.push(function (plot: any, eventHolder: any) {
      eventHolder.unbind('mousemove', onMouseMove);
      eventHolder.unbind('mousedown', onMouseDown);

      if (mouseUpHandler) $(document).unbind('mouseup', mouseUpHandler);
    });
  }

  $.plot.plugins.push({
    init,
    options: {
      selection: {
        mode: null, // one of null, "x", "y" or "xy"
        color: '#e8cfac',
        shape: 'round', // one of "round", "miter", or "bevel"
        minSize: 5, // minimum number of pixels
      },
    },
    name: 'selection',
    version: '1.1',
  });
  // @ts-ignore
})(jQuery);
