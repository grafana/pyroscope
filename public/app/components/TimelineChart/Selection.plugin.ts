/* eslint eqeqeq: "off" 
  -- TODO: Initial logic used == instead of ===  */
// extending logic of Flot's selection plugin (react-flot/flot/jquery.flot.selection)
import clamp from './clamp';
import extractRange from './extractRange';

const handleWidth = 4;
const handleHeight = 22;

interface IPlot extends jquery.flot.plot, jquery.flot.plotOptions {
  clearSelection: (preventEvent: boolean) => void;
  getSelection: () => void;
}

interface IFlotOptions extends jquery.flot.plotOptions {
  selection?: {
    selectionType: 'single' | 'double';
    mode?: 'x' | 'y';
    minSize: number;
    boundaryColor?: string;
    overlayColor?: string;
    shape: CanvasLineJoin;
    color: string;
    selectionWithHandler: boolean;
  };
}

type EventType = { pageX: number; pageY: number; which?: number };

(function ($) {
  function init(plot: IPlot) {
    const placeholder = plot.getPlaceholder();
    const selection = {
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
    const savedhandlers: ShamefulAny = {};

    let mouseUpHandler: ShamefulAny = null;

    function getCursorPositionX(e: EventType) {
      const plotOffset = plot.getPlotOffset();
      const offset = placeholder.offset();
      return clamp(0, plot.width(), e.pageX - offset!.left - plotOffset.left);
    }

    function getPlotSelection() {
      // unlike function getSelection() which shows temp selection (it doesnt save any data between rerenders)
      // this function returns left X and right X coords of visible user selection (translates opts.grid.markings to X coords)
      const o = plot.getOptions();
      const plotOffset = plot.getPlotOffset();
      const extractedX = extractRange(plot, 'x');

      return {
        left:
          Math.floor(extractedX.axis.p2c(o.grid!.markings[0]?.xaxis.from)) +
          plotOffset.left,
        right:
          Math.floor(extractedX.axis.p2c(o.grid!.markings[0]?.xaxis.to)) +
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

      if (isLeftSelecting) {
        return 'left';
      }
      if (isRightSelecting) {
        return 'right';
      }
      return null;
    }

    function setCursor(type: string) {
      $('canvas.flot-overlay').css('cursor', type);
    }

    function onMouseMove(e: EventType) {
      const options: IFlotOptions = plot.getOptions();

      if (options?.selection?.selectionType === 'single') {
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

        placeholder.trigger('plotselecting', [getSelection()]);
      }
    }

    function onMouseDown(e: EventType) {
      const options: IFlotOptions = plot.getOptions();

      if (e.which != 1) {
        // only accept left-click
        return;
      }

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

      if (options?.selection?.selectionType === 'single') {
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

        const offset = placeholder.offset();
        const plotOffset = plot.getPlotOffset();

        if (dragSide === 'right') {
          setSelectionPos(selection.first, {
            pageX: left - plotOffset.left + offset!.left + plotOffset.left,
          } as EventType);
        } else if (dragSide === 'left') {
          setSelectionPos(selection.first, {
            pageX: right - plotOffset.left + offset!.left + plotOffset.left,
          } as EventType);
        } else {
          setSelectionPos(selection.first, e);
        }

        (selection.selectingSide as 'left' | 'right' | null) = dragSide;
      } else {
        setSelectionPos(selection.first, e);
      }

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
      if (document.onselectstart !== undefined) {
        document.onselectstart = savedhandlers.onselectstart;
      }
      if (document.ondrag !== undefined) {
        document.ondrag = savedhandlers.ondrag;
      }

      // no more dragging
      selection.active = false;
      updateSelection(e);

      if (selectionIsSane()) {
        triggerSelectedEvent();
      } else {
        // this counts as a clear
        placeholder.trigger('plotunselected', []);
        placeholder.trigger('plotselecting', [null]);
      }

      setCursor('crosshair');

      return false;
    }

    function getSelection() {
      if (!selectionIsSane()) {
        return null;
      }

      if (!selection.show) {
        return null;
      }

      const r: ShamefulAny = {};
      const c1 = selection.first;
      const c2 = selection.second;
      $.each(plot.getAxes(), function (name, axis: ShamefulAny) {
        if (axis.used) {
          const p1 = axis.c2p(c1[axis.direction as 'x' | 'y']);
          const p2 = axis.c2p(c2[axis.direction as 'x' | 'y']);
          r[name] = { from: Math.min(p1, p2), to: Math.max(p1, p2) };
        }
      });
      return r;
    }

    function triggerSelectedEvent() {
      const r = getSelection();

      placeholder.trigger('plotselected', [r]);

      // backwards-compat stuff, to be removed in future
      if (r.xaxis && r.yaxis) {
        placeholder.trigger('selected', [
          {
            x1: r.xaxis.from,
            y1: r.yaxis.from,
            x2: r.xaxis.to,
            y2: r.yaxis.to,
          },
        ]);
      }
    }

    function setSelectionPos(pos: { x: number; y: number }, e: EventType) {
      const options: IFlotOptions = plot.getOptions();
      const offset = placeholder.offset();
      const plotOffset = plot.getPlotOffset();
      pos.x = clamp(0, plot.width(), e.pageX - offset!.left - plotOffset.left);
      pos.y = clamp(0, plot.height(), e.pageY - offset!.top - plotOffset.top);

      if (options?.selection?.mode == 'y') {
        pos.x = pos == selection.first ? 0 : plot.width();
      }

      if (options?.selection?.mode == 'x') {
        pos.y = pos == selection.first ? 0 : plot.height();
      }
    }

    function updateSelection(pos: EventType) {
      if (pos.pageX == null) {
        return;
      }

      setSelectionPos(selection.second, pos);
      if (selectionIsSane()) {
        selection.show = true;
        plot.triggerRedrawOverlay();
      } else {
        clearSelection(true);
      }
    }

    function clearSelection(preventEvent: boolean) {
      if (selection.show) {
        selection.show = false;
        plot.triggerRedrawOverlay();
        if (!preventEvent) {
          placeholder.trigger('plotunselected', []);
        }
      }
    }

    function selectionIsSane() {
      const options: IFlotOptions = plot.getOptions();
      const minSize = options?.selection?.minSize || 5;

      return (
        Math.abs(selection.second.x - selection.first.x) >= minSize &&
        Math.abs(selection.second.y - selection.first.y) >= minSize
      );
    }

    plot.clearSelection = clearSelection;
    plot.getSelection = getSelection;

    plot.hooks!.bindEvents!.push(function (plot, eventHolder) {
      const options: IFlotOptions = plot.getOptions();
      if (options?.selection?.mode != null) {
        eventHolder.mousemove(onMouseMove);
        eventHolder.mousedown(onMouseDown);
      }
    });

    plot.hooks!.drawOverlay!.push(function (plot, ctx) {
      // draw selection
      if (selection.show && selectionIsSane()) {
        const plotOffset = plot.getPlotOffset();
        const options: IFlotOptions = plot.getOptions();

        ctx.save();
        ctx.translate(plotOffset.left, plotOffset.top);

        const c = ($ as ShamefulAny).color.parse(options?.selection?.color);

        ctx.strokeStyle = c.scale('a', 0.8).toString();
        ctx.lineWidth = 1;
        ctx.lineJoin = options.selection!.shape;
        ctx.fillStyle = c.scale('a', 0.4).toString();

        const x = Math.min(selection.first.x, selection.second.x) + 0.5;
        const y = Math.min(selection.first.y, selection.second.y) + 0.5;
        const w = Math.abs(selection.second.x - selection.first.x) - 1;
        const h = Math.abs(selection.second.y - selection.first.y) - 1;

        if (selection.selectingSide) {
          ctx.fillStyle = options?.selection?.overlayColor || 'transparent';
          ctx.fillRect(x, y, w, h);
          drawHorizontalSelectionLines({
            ctx,
            opts: options,
            leftX: x,
            rightX: x + w,
            yMax: h,
            yMin: 0,
          });
          drawVerticalSelectionLines({
            ctx,
            opts: options,
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
            options?.selection?.boundaryColor
          );
        } else {
          ctx.fillRect(x, y, w, h);
          ctx.strokeRect(x, y, w, h);
        }

        ctx.restore();
      }
    });

    plot.hooks!.draw!.push(function (plot, ctx) {
      const options: IFlotOptions = plot.getOptions();

      if (
        options?.selection?.selectionType === 'single' &&
        options?.selection?.selectionWithHandler
      ) {
        const plotOffset = plot.getPlotOffset();
        const extractedY = extractRange(plot, 'y');
        const { left, right } = getPlotSelection();

        const yMax =
          Math.floor(extractedY.axis.p2c(extractedY.axis.min)) + plotOffset.top;
        const yMin = 0 + plotOffset.top;

        // draw selection overlay
        ctx.fillStyle = options.selection.overlayColor || 'transparent';
        ctx.fillRect(left, yMin, right - left, yMax - plotOffset.top);

        drawHorizontalSelectionLines({
          ctx,
          opts: options,
          leftX: left,
          rightX: right,
          yMax,
          yMin,
        });
        drawVerticalSelectionLines({
          ctx,
          opts: options,
          leftX: left + 0.5,
          rightX: right - 0.5,
          yMax,
          yMin: yMin + 4,
          drawHandles: true,
        });
      }
    });

    plot.hooks!.shutdown!.push(function (plot, eventHolder) {
      eventHolder.unbind('mousemove', onMouseMove);
      eventHolder.unbind('mousedown', onMouseDown);

      if (mouseUpHandler) {
        $(document).unbind('mouseup', mouseUpHandler);
      }
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
    // left line
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

    // right line
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
  fillColor?: string
) {
  const K = (4 * (Math.SQRT2 - 1)) / 3;
  const right = left + width;
  const bottom = top + height;
  ctx.beginPath();
  ctx.setLineDash([]);
  // top left
  ctx.moveTo(left + radius, top);
  // top right
  ctx.lineTo(right - radius, top);
  // right top
  ctx.bezierCurveTo(
    right + radius * (K - 1),
    top,
    right,
    top + radius * (1 - K),
    right,
    top + radius
  );
  // right bottom
  ctx.lineTo(right, bottom - radius);
  // bottom right
  ctx.bezierCurveTo(
    right,
    bottom + radius * (K - 1),
    right + radius * (K - 1),
    bottom,
    right - radius,
    bottom
  );
  // bottom left
  ctx.lineTo(left + radius, bottom);
  // left bottom
  ctx.bezierCurveTo(
    left + radius * (1 - K),
    bottom,
    left,
    bottom + radius * (K - 1),
    left,
    bottom - radius
  );
  // left top
  ctx.lineTo(left, top + radius);
  // top left again
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
