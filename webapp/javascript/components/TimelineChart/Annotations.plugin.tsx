import React from 'react';
import * as ReactDOM from 'react-dom';
import Color from 'color';
import { randomId } from '@webapp/util/randomId';
import { CtxType } from './types';
import extractRange from './extractRange';

const AnnotationMsgIcon =
  require('../../../images/annotationMessage.png').default;

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
      return AnnotationMsgIcon;
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
  plot: jquery.flot.plot,
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

function drawRoundRect(
  ctx: CtxType,
  x: number,
  y: number,
  w: number,
  h: number,
  radius: number,
  color: string | Color
) {
  const r = x + w;
  const b = y + h;
  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.fillStyle = color;
  ctx.lineWidth = 1;
  ctx.moveTo(x + radius, y);
  ctx.lineTo(r - radius, y);
  ctx.quadraticCurveTo(r, y, r, y + radius);
  ctx.lineTo(r, y + h - radius);
  ctx.quadraticCurveTo(r, b, r - radius, b);
  ctx.lineTo(x + radius, b);
  ctx.quadraticCurveTo(x, b, x, b - radius);
  ctx.lineTo(x, y + radius);
  ctx.quadraticCurveTo(x, y, x + radius, y);
  ctx.fill();
  ctx.stroke();
}

(function ($) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const annotationsPositions: AnnotationPosition[] = [];
    const placeholder = plot.getPlaceholder();

    function onHover(_: unknown, pos: { x: number; y: number }) {
      if (annotationsPositions?.length) {
        const { x, y } = getCursorPositionInPx(plot, pos);

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

      const { x, y } = getCursorPositionInPx(plot, pos);

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
        const extractedX = extractRange(plot, axes, 'x');
        const extractedY = extractRange(plot, axes, 'y');

        o.annotations.forEach((a: AnnotationType) => {
          const left: number =
            Math.floor(extractedX.axis.p2c(a.timestamp * 1000)) +
            plotOffset.left;
          const yMax =
            Math.floor(extractedY.axis.p2c(extractedY.axis.min)) +
            plotOffset.top;
          const yMin = 0 + plotOffset.top;
          const lineWidth = 1;
          const subPixel = lineWidth / 2 || 0;
          const squareHeight = 23;
          const squareWidth = 26;

          // draw vertical line
          ctx.beginPath();
          ctx.strokeStyle = a.color;
          ctx.lineWidth = lineWidth;
          ctx.moveTo(left + subPixel, yMax);
          ctx.lineTo(left + subPixel, yMin);
          ctx.stroke();

          // draw icon rounded square
          const rectParams = {
            fromX: left - squareWidth / 2,
            toX: left + squareWidth / 2,
            fromY: 0,
            toY: squareHeight,
            ...a,
          };
          drawRoundRect(
            ctx,
            rectParams.fromX,
            rectParams.fromY,
            squareWidth,
            squareHeight,
            3,
            a.color
          );
          annotationsPositions.push(rectParams);

          // draw icon
          const img = new Image();
          img.onload = () => {
            ctx.drawImage(
              img,
              left - squareWidth / 2 + 3,
              2,
              squareHeight - 3,
              squareHeight - 3
            );
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
