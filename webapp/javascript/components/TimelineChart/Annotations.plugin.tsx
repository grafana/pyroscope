import React from 'react';
import * as ReactDOM from 'react-dom';
import { randomId } from '@webapp/util/randomId';
import { PlotType, CtxType } from './types';
import extractRange from './extractRange';

type AnnotationType = {
  content: string;
  timestamp: number;
  type: 'message';
  color: string;
};

interface IFlotOptions extends jquery.flot.plotOptions {
  annotations?: AnnotationType[];
  ContextMenu?: React.FC<ContextMenuProps>;
}

type AnnotationPosition = {
  fromX: number;
  toX: number;
  fromY: number;
  toY: number;
  timestamp: number;
  content: string;
};

export interface ContextMenuProps {
  click: {
    /** The X position in the window where the click originated */
    pageX: number;
    /** The Y position in the window where the click originated */
    pageY: number;
  };
  timestamp: number;
  containerEl: HTMLElement;
  value?: {
    timestamp: number;
    content: string;
  } | null;
}

const WRAPPER_ID = randomId('contextMenu');

const getIconByAnnotationType = (type: string) => {
  switch (type) {
    case 'message':
    default:
      return 'data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4NCjwhLS0gU3ZnIFZlY3RvciBJY29ucyA6IGh0dHA6Ly93d3cub25saW5ld2ViZm9udHMuY29tL2ljb24gLS0+DQo8IURPQ1RZUEUgc3ZnIFBVQkxJQyAiLS8vVzNDLy9EVEQgU1ZHIDEuMS8vRU4iICJodHRwOi8vd3d3LnczLm9yZy9HcmFwaGljcy9TVkcvMS4xL0RURC9zdmcxMS5kdGQiPg0KPHN2ZyB2ZXJzaW9uPSIxLjEiIHhtbG5zPSJodHRwOi8vd3d3LnczLm9yZy8yMDAwL3N2ZyIgeG1sbnM6eGxpbms9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkveGxpbmsiIHg9IjBweCIgeT0iMHB4Ig0KICAgIHZpZXdCb3g9IjAgMCAxMDAwIDEwMDAiIGVuYWJsZS1iYWNrZ3JvdW5kPSJuZXcgMCAwIDEwMDAgMTAwMCIgeG1sOnNwYWNlPSJwcmVzZXJ2ZSI+DQogICAgPG1ldGFkYXRhPiBTdmcgVmVjdG9yIEljb25zIDogaHR0cDovL3d3dy5vbmxpbmV3ZWJmb250cy5jb20vaWNvbiA8L21ldGFkYXRhPg0KICAgIDxnPg0KICAgICAgICA8cGF0aCBmaWxsPSIjZmZmIg0KICAgICAgICAgICAgZD0iTTg5Miw4MTguNGgtNzkuM2wtNzAuOCwxMjIuN0w1MjkuNCw4MTguNEgxMDhjLTU0LjEsMC05OC00My45LTk4LTk4VjE1Ni45YzAtNTQuMSw0My45LTk4LDk4LTk4aDc4NGM1NC4xLDAsOTgsNDMuOSw5OCw5OHY1NjMuNUM5OTAsNzc0LjUsOTQ2LjEsODE4LjQsODkyLDgxOC40eiBNOTE2LjUsMTMyLjRoLTgzM3Y2MTIuNWg0NjMuOWwxNzAuMSw5OC4ybDU2LjctOTguMmgxNDIuNFYxMzIuNHogTTE4MS41LDU4NS43YzAtMjAuMywxNi41LTM2LjgsMzYuOC0zNi44aDU2My41YzIwLjMsMCwzNi44LDE2LjUsMzYuOCwzNi44YzAsMjAuMy0xNi41LDM2LjgtMzYuOCwzNi44SDIxOC4zQzE5OCw2MjIuNCwxODEuNSw2MDYsMTgxLjUsNTg1Ljd6IE03ODEuOCw0NzUuNEgyMTguM2MtMjAuMywwLTM2LjgtMTYuNS0zNi44LTM2LjhjMC0yMC4zLDE2LjUtMzYuOCwzNi44LTM2LjhoNTYzLjVjMjAuMywwLDM2LjgsMTYuNSwzNi44LDM2LjhDODE4LjUsNDU5LDgwMiw0NzUuNCw3ODEuOCw0NzUuNHogTTU4NS44LDMyOC40SDIxOC4zYy0yMC4zLDAtMzYuOC0xNi41LTM2LjgtMzYuN2MwLTIwLjMsMTYuNS0zNi44LDM2LjgtMzYuOGgzNjcuNWMyMC4zLDAsMzYuOCwxNi41LDM2LjgsMzYuOEM2MjIuNSwzMTIsNjA2LDMyOC40LDU4NS44LDMyOC40eiIgLz4NCiAgICA8L2c+DQo8L3N2Zz4=';
  }
};

const shouldStartAnnotationsFunctionality = (annotations?: AnnotationType[]) =>
  Array.isArray(annotations);

const inject = ($: JQueryStatic) => {
  const alreadyInitialized = $(`#${WRAPPER_ID}`).length > 0;

  if (alreadyInitialized) {
    return $(`#${WRAPPER_ID}`);
  }

  const body = $('body');
  return $(`<div id="${WRAPPER_ID}" />`).appendTo(body);
};

const getCursorPositionInPx = (
  plot: PlotType,
  positionInTimestamp: { x: number; y: number }
) => {
  const axes = plot.getAxes();
  const extractedX = extractRange(plot, axes, 'x');
  const extractedY = extractRange(plot, axes, 'y');
  const plotOffset = plot.getPlotOffset() as {
    top: number;
    left: number;
  };

  return {
    x: Math.floor(extractedX.axis.p2c(positionInTimestamp.x)) + plotOffset.left,
    y: Math.floor(extractedY.axis.p2c(positionInTimestamp.y)) + plotOffset.top,
  };
};

const findAnnotationByCursorPosition = (
  x: number,
  y: number,
  list: AnnotationPosition[] = []
) => {
  return list?.find((an) => {
    return x >= an.fromX && x <= an.toX && y >= an.fromY && y <= an.toY;
  });
};

(function ($) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const annotationsPositions: AnnotationPosition[] = [];
    const placeholder = plot.getPlaceholder();

    function onHover(_: unknown, pos: { x: number; y: number }) {
      if (annotationsPositions?.length) {
        const { x, y } = getCursorPositionInPx(
          plot as PlotType & jquery.flot.plot,
          pos
        );

        const annotation = findAnnotationByCursorPosition(
          x,
          y,
          annotationsPositions
        );

        if (annotation) {
          placeholder.trigger('hoveringOnAnnotation', [{ hovering: true }]);
        } else {
          placeholder.trigger('hoveringOnAnnotation', [{ hovering: false }]);
        }
      }
    }

    function onClick(
      _: unknown,
      pos: { x: number; pageX: number; pageY: number; y: number }
    ) {
      const options: IFlotOptions = plot.getOptions();
      const container = inject($);
      const containerEl = container?.[0];

      ReactDOM.unmountComponentAtNode(containerEl);

      const ContextMenu = options?.ContextMenu;

      const { x, y } = getCursorPositionInPx(
        plot as PlotType & jquery.flot.plot,
        pos
      );

      const annotation = findAnnotationByCursorPosition(
        x,
        y,
        annotationsPositions
      );

      if (ContextMenu && containerEl) {
        const timestamp = Math.round(pos.x / 1000);
        const value = annotation
          ? {
              timestamp: annotation.timestamp,
              content: annotation.content,
            }
          : null;

        ReactDOM.render(
          <ContextMenu
            click={{ ...pos }}
            containerEl={containerEl}
            timestamp={timestamp}
            value={value}
          />,
          containerEl
        );
      }
    }

    plot.hooks!.draw!.push((plot, ctx: CtxType) => {
      const o: IFlotOptions = plot.getOptions();

      if (o.annotations?.length) {
        const axes = plot.getAxes();
        const plotOffset: { top: number; left: number } = plot.getPlotOffset();
        const extractedX = extractRange(
          plot as PlotType & jquery.flot.plot,
          axes,
          'x'
        );
        const extractedY = extractRange(
          plot as PlotType & jquery.flot.plot,
          axes,
          'y'
        );

        o.annotations.forEach((a: AnnotationType) => {
          const left: number =
            Math.floor(extractedX.axis.p2c(a.timestamp * 1000)) +
            plotOffset.left;
          const yMax =
            Math.floor(extractedY.axis.p2c(extractedY.axis.min)) +
            plotOffset.top;
          const yMin = 0 + plotOffset.top;
          const lineWidth = 2;
          const subPixel = lineWidth / 2 || 0;
          const squareHeight = 30;
          const squareWidth = 34;

          // draw vertical line
          ctx.beginPath();
          ctx.strokeStyle = a.color;
          ctx.lineWidth = lineWidth;
          ctx.moveTo(left + subPixel, yMax);
          ctx.lineTo(left + subPixel, yMin);
          ctx.stroke();

          // draw icon square
          ctx.beginPath();
          ctx.fillStyle = a.color;
          const rectParams = {
            fromX: left + 1 - squareWidth / 2,
            toX: left + 1 + squareWidth / 2,
            fromY: 0,
            toY: squareHeight,
            ...a,
          };
          ctx.fillRect(
            rectParams.fromX,
            rectParams.fromY,
            squareWidth,
            squareHeight
          );
          ctx.stroke();
          annotationsPositions.push(rectParams);

          // draw icon
          const img = new Image();
          img.onload = () => {
            ctx.drawImage(img, left - squareWidth / 2 + 4, 1, 28, 28);
          };

          img.src = getIconByAnnotationType(a.type);
        });
      }
    });

    plot.hooks!.bindEvents!.push((plot) => {
      const o: IFlotOptions = plot.getOptions();

      if (shouldStartAnnotationsFunctionality(o.annotations)) {
        placeholder.bind('plothover', onHover);
        placeholder.bind('plotclick', onClick);
      }
    });

    plot.hooks!.shutdown!.push((plot) => {
      const o: IFlotOptions = plot.getOptions();

      if (shouldStartAnnotationsFunctionality(o.annotations)) {
        placeholder.unbind('plothover', onHover);
        placeholder.unbind('plotclick', onClick);

        const container = inject($);

        ReactDOM.unmountComponentAtNode(container?.[0]);
      }
    });
  }

  $.plot.plugins.push({
    init,
    options: {},
    name: 'annotations',
    version: '1.0',
  });
})(jQuery);
