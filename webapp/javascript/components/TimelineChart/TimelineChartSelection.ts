/* eslint-disable */
// extending logic of Flot's selection plugin (react-flot/flot/jquery.flot.selection)

type PlotType = {
  getPlotOffset: () => any;
  getOptions: () => any;
  getAxes: () => any[];
  getXAxes: () => any[];
  getYAxes: () => any[];
  getPlaceholder: () => {
    trigger: (arg0: string, arg1: any[]) => void;
    offset: () => {
      left: number;
      top: number;
    };
  };
  triggerRedrawOverlay: () => void;
  width: () => number;
  height: () => number;
  clearSelection: (preventEvent: boolean) => void;
  setSelection: (ranges: any, preventEvent: any) => void;
  getSelection: () => {} | null;
  hooks: any;
};

type CtxType = {
  save: () => void;
  translate: (arg0: any, arg1: any) => void;
  strokeStyle: any;
  lineWidth: number;
  lineJoin: any;
  fillStyle: any;
  fillRect: (arg0: number, arg1: number, arg2: number, arg3: number) => void;
  strokeRect: (arg0: number, arg1: number, arg2: number, arg3: number) => void;
  restore: () => void;
};

type EventHolderType = {
  unbind: (arg0: string, arg1: { (e: any): void; (e: any): void }) => void;
  mousemove: (arg0: (e: EventType) => void) => void;
  mousedown: (arg0: (e: EventType) => void) => void;
};

type EventType = { pageX: number; pageY: number; which?: number };

const handleWidth = 4;
const handleHeight = 22;

(function ($) {
  function init(plot: PlotType) {
    var selection = {
      first: { x: -1, y: -1 },
      second: { x: -1, y: -1 },
      show: false,
      active: false,
      selectingSide: null,
    };

    // FIXME: The drag handling implemented here should be
    // abstracted out, there's some similar code from a library in
    // the navigation plugin, this should be massaged a bit to fit
    // the Flot cases here better and reused. Doing this would
    // make this plugin much slimmer.
    var savedhandlers: any = {};

    var mouseUpHandler:
      | boolean
      | JQuery.TypeEventHandler<
          Document,
          undefined,
          Document,
          Document,
          'mouseup'
        >
      | null = null;

    function getCursorPositionX(e: EventType) {
      const plotOffset = plot.getPlotOffset();
      const offset = plot.getPlaceholder().offset();
      return clamp(0, e.pageX - offset.left - plotOffset.left, plot.width());
    }

    function getPlotSelection() {
      // unlike function getSelection() which shows temp selection (it doesnt save any data between rerenders)
      // this function returns left X and right X coords of visible user selection (translates opts.grid.markings to X coords)
      const o = plot.getOptions();
      const axes = plot.getAxes();
      const plotOffset = plot.getPlotOffset();
      const extractedX = extractRange(axes, 'x');

      return {
        left:
          Math.floor(extractedX.axis.p2c(o.grid.markings[0]?.xaxis.from)) +
          plotOffset.left,
        right:
          Math.floor(extractedX.axis.p2c(o.grid.markings[0]?.xaxis.to)) +
          plotOffset.left,
      };
    }

    function getDragSide({
      x,
      leftSelectionX,
      rightSelectionX,
    }: {
      x: number;
      leftSelectionX: number;
      rightSelectionX: number;
    }) {
      const plotOffset = plot.getPlotOffset();
      const isLeftSelecting =
        Math.abs(x + plotOffset.left - leftSelectionX) <= 5;
      const isRightSelecting =
        Math.abs(x + plotOffset.left - rightSelectionX) <= 5;

      return isLeftSelecting ? 'left' : isRightSelecting ? 'right' : null;
    }

    function setCursor(type: string) {
      $('canvas.flot-overlay').css('cursor', type);
    }

    function onMouseMove(e: EventType) {
      const o = plot.getOptions();

      if (o?.selection?.selectionType === 'single') {
        const { left, right } = getPlotSelection();
        const clickX = getCursorPositionX(e);
        const dragSide = getDragSide({
          x: clickX,
          leftSelectionX: left,
          rightSelectionX: right,
        });

        if (dragSide) {
          setCursor('grab');
        } else {
          setCursor('crosshair');
        }
      }

      if (selection.active) {
        updateSelection(e);

        if (selection.selectingSide) {
          setCursor('grabbing');
        } else {
          setCursor('crosshair');
        }

        plot.getPlaceholder().trigger('plotselecting', [getSelection()]);
      }
    }

    function onMouseDown(e: EventType) {
      if (e.which != 1)
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

      const offset = plot.getPlaceholder().offset();
      const plotOffset = plot.getPlotOffset();
      const { left, right } = getPlotSelection();
      const clickX = getCursorPositionX(e);
      const dragSide = getDragSide({
        x: clickX,
        leftSelectionX: left,
        rightSelectionX: right,
      });

      if (dragSide) {
        setCursor('grabbing');
      }

      if (dragSide === 'right') {
        setSelectionPos(selection.first, {
          pageX: left - plotOffset.left + offset.left + plotOffset.left,
        } as EventType);
      } else if (dragSide === 'left') {
        setSelectionPos(selection.first, {
          pageX: right - plotOffset.left + offset.left + plotOffset.left,
        } as EventType);
      } else {
        setSelectionPos(selection.first, e);
      }

      (selection.selectingSide as 'left' | 'right' | null) = dragSide;
      selection.active = true;

      // this is a bit silly, but we have to use a closure to be
      // able to whack the same handler again
      mouseUpHandler = function (e: EventType) {
        onMouseUp(e);
      };

      $(document).one('mouseup', mouseUpHandler);
    }

    function onMouseUp(e: EventType) {
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

      setCursor('crosshair');

      return false;
    }

    function getSelection() {
      if (!selectionIsSane()) return null;

      if (!selection.show) return null;

      var r: any = {},
        c1: any = selection.first,
        c2: any = selection.second;
      $.each(plot.getAxes(), function (name, axis) {
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

    function clamp(min: number, value: number, max: number) {
      return value < min ? min : value > max ? max : value;
    }

    function setSelectionPos(pos: { x: number; y: number }, e: EventType) {
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

    function updateSelection(pos: EventType) {
      if (pos.pageX == null) return;

      setSelectionPos(selection.second, pos);
      if (selectionIsSane()) {
        selection.show = true;
        plot.triggerRedrawOverlay();
      } else clearSelection(true);
    }

    function clearSelection(preventEvent: boolean) {
      if (selection.show) {
        selection.show = false;
        plot.triggerRedrawOverlay();
        if (!preventEvent) plot.getPlaceholder().trigger('plotunselected', []);
      }
    }

    // function taken from markings support in Flot
    function extractRange(ranges: { [x: string]: any }, coord: string) {
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
      if (!ranges[key as string]) {
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

    plot.hooks.bindEvents.push(function (
      plot: PlotType,
      eventHolder: EventHolderType
    ) {
      var o = plot.getOptions();
      if (o.selection.mode != null) {
        eventHolder.mousemove(onMouseMove);
        eventHolder.mousedown(onMouseDown);
      }
    });

    plot.hooks.drawOverlay.push(function (plot: PlotType, ctx: CtxType) {
      // draw selection
      if (selection.show && selectionIsSane()) {
        const plotOffset = plot.getPlotOffset();
        const o = plot.getOptions();

        ctx.save();
        ctx.translate(plotOffset.left, plotOffset.top);

        var c = ($ as any).color.parse(o.selection.color);

        ctx.strokeStyle = c.scale('a', 0.8).toString();
        ctx.lineWidth = 1;
        ctx.lineJoin = o.selection.shape;
        ctx.fillStyle = c.scale('a', 0.4).toString();

        var x = Math.min(selection.first.x, selection.second.x) + 0.5,
          y = Math.min(selection.first.y, selection.second.y) + 0.5,
          w = Math.abs(selection.second.x - selection.first.x) - 1,
          h = Math.abs(selection.second.y - selection.first.y) - 1;

        if (selection.selectingSide) {
          ctx.fillStyle = o.selection.overlayColor || 'transparent';
          ctx.fillRect(x, y, w, h);
          drawHorizontalSelectionLines({
            ctx,
            opts: o,
            leftX: x,
            rightX: x + w,
            yMax: h,
            yMin: 0,
          });
          drawVerticalSelectionLines({
            ctx,
            opts: o,
            leftX: x,
            rightX: x + w,
            yMax: h,
            yMin: 0,
            drawHandles: false,
          });

          drawRoundedRect(
            ctx,
            (selection.selectingSide === 'left' ? x : x + w) -
              handleWidth / 2 +
              0.5,
            h / 2 - handleHeight / 2 - 1,
            handleWidth,
            handleHeight,
            2,
            o.selection.boundaryColor
          );
        } else {
          ctx.fillRect(x, y, w, h);
          ctx.strokeRect(x, y, w, h);
        }

        ctx.restore();
      }
    });

    plot.hooks.draw.push(function (plot: PlotType, ctx: CtxType) {
      const opts = plot.getOptions();

      if (opts?.selection?.selectionType === 'single') {
        const axes = plot.getAxes();
        const plotOffset = plot.getPlotOffset();
        const extractedY = extractRange(axes, 'y');
        const { left, right } = getPlotSelection();

        const yMax =
          Math.floor(extractedY.axis.p2c(extractedY.axis.min)) + plotOffset.top;
        const yMin = 0 + plotOffset.top;

        // draw selection overlay
        ctx.fillStyle = opts.selection.overlayColor || 'transparent';
        ctx.fillRect(left, yMin, right - left, yMax - plotOffset.top);

        drawHorizontalSelectionLines({
          ctx,
          opts,
          leftX: left,
          rightX: right,
          yMax,
          yMin,
        });
        drawVerticalSelectionLines({
          ctx,
          opts,
          leftX: left + 0.5,
          rightX: right - 0.5,
          yMax,
          yMin: yMin + 4,
          drawHandles: true,
        });
      }
    });

    plot.hooks.shutdown.push(function (
      plot: PlotType,
      eventHolder: EventHolderType
    ) {
      eventHolder.unbind('mousemove', onMouseMove);
      eventHolder.unbind('mousedown', onMouseDown);

      if (mouseUpHandler)
        ($ as any)(document).unbind('mouseup', mouseUpHandler);
    });
  }

  ($ as any).plot.plugins.push({
    init: init,
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
})(jQuery);

const drawVerticalSelectionLines = ({
  ctx,
  opts,
  leftX,
  rightX,
  yMax,
  yMin,
  drawHandles,
}: {
  ctx: ShamefulAny;
  opts: ShamefulAny;
  leftX: number;
  rightX: number;
  yMax: number;
  yMin: number;
  drawHandles: boolean;
}) => {
  if (leftX && rightX && yMax) {
    const lineWidth =
      opts.grid.markings?.[opts.grid.markings?.length - 1].lineWidth || 1;
    const subPixel = lineWidth / 2 || 0;
    //left line
    ctx.beginPath();
    ctx.strokeStyle = opts.selection.boundaryColor;
    ctx.lineWidth = lineWidth;

    if (opts?.selection?.selectionType === 'single') {
      ctx.setLineDash([2]);
    }

    ctx.moveTo(leftX + subPixel, yMax);
    ctx.lineTo(leftX + subPixel, yMin);
    ctx.stroke();

    if (drawHandles) {
      drawRoundedRect(
        ctx,
        leftX - handleWidth / 2 + subPixel,
        yMax / 2 - handleHeight / 2 + 3,
        handleWidth,
        handleHeight,
        2,
        opts.selection.boundaryColor
      );
    }

    //right line
    ctx.beginPath();
    ctx.strokeStyle = opts.selection.boundaryColor;
    ctx.lineWidth = lineWidth;

    if (opts?.selection?.selectionType === 'single') {
      ctx.setLineDash([2]);
    }

    ctx.moveTo(rightX + subPixel, yMax);
    ctx.lineTo(rightX + subPixel, yMin);
    ctx.stroke();

    if (drawHandles) {
      drawRoundedRect(
        ctx,
        rightX - handleWidth / 2 + subPixel,
        yMax / 2 - handleHeight / 2 + 3,
        handleWidth,
        handleHeight,
        2,
        opts.selection.boundaryColor
      );
    }
  }
};

const drawHorizontalSelectionLines = ({
  ctx,
  opts,
  leftX,
  rightX,
  yMax,
  yMin,
}: {
  ctx: ShamefulAny;
  opts: ShamefulAny;
  leftX: number;
  rightX: number;
  yMax: number;
  yMin: number;
}) => {
  if (leftX && rightX && yMax) {
    const topLineWidth = 4;
    const lineWidth =
      opts.grid.markings?.[opts.grid.markings?.length - 1].lineWidth || 1;
    const subPixel = lineWidth / 2 || 0;

    // top line
    ctx.beginPath();
    ctx.strokeStyle = opts.selection.boundaryColor;
    ctx.lineWidth = topLineWidth;
    ctx.setLineDash([]);
    ctx.moveTo(rightX + subPixel, yMin + topLineWidth / 2);
    ctx.lineTo(leftX + subPixel, yMin + topLineWidth / 2);
    ctx.stroke();

    // bottom line
    ctx.beginPath();
    ctx.strokeStyle = opts.selection.boundaryColor;
    ctx.lineWidth = lineWidth;
    ctx.setLineDash([2]);
    ctx.moveTo(rightX + subPixel, yMax);
    ctx.lineTo(leftX + subPixel, yMax);
    ctx.stroke();
  }
};

function drawRoundedRect(
  ctx: ShamefulAny,
  left: number,
  top: number,
  width: number,
  height: number,
  radius: number,
  fillColor: string
) {
  var K = (4 * (Math.SQRT2 - 1)) / 3;
  var right = left + width;
  var bottom = top + height;
  ctx.beginPath();
  ctx.setLineDash([]);
  // top left
  ctx.moveTo(left + radius, top);
  // top right
  ctx.lineTo(right - radius, top);
  //right top
  ctx.bezierCurveTo(
    right + radius * (K - 1),
    top,
    right,
    top + radius * (1 - K),
    right,
    top + radius
  );
  //right bottom
  ctx.lineTo(right, bottom - radius);
  //bottom right
  ctx.bezierCurveTo(
    right,
    bottom + radius * (K - 1),
    right + radius * (K - 1),
    bottom,
    right - radius,
    bottom
  );
  //bottom left
  ctx.lineTo(left + radius, bottom);
  //left bottom
  ctx.bezierCurveTo(
    left + radius * (1 - K),
    bottom,
    left,
    bottom + radius * (K - 1),
    left,
    bottom - radius
  );
  //left top
  ctx.lineTo(left, top + radius);
  //top left again
  ctx.bezierCurveTo(
    left,
    top + radius * (1 - K),
    left + radius * (1 - K),
    top,
    left + radius,
    top
  );
  ctx.lineWidth = 1;
  ctx.strokeStyle = fillColor;
  ctx.fillStyle = fillColor;
  ctx.fill();
  ctx.stroke();
}
