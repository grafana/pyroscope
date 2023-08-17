import React from 'react';
import { createRoot } from 'react-dom/client';
import Color from 'color';
import { Provider } from 'react-redux';
import store from '@pyroscope/redux/store';
import extractRange from './extractRange';
import AnnotationMark from './AnnotationMark';

type AnnotationType = {
  content: string;
  timestamp: number;
  type: 'message';
  color: Color;
};

interface IFlotOptions extends jquery.flot.plotOptions {
  annotations?: AnnotationType[];
  wrapperId?: string;
}

interface IPlot extends jquery.flot.plot, jquery.flot.plotOptions {}

(function ($) {
  function init(plot: IPlot) {
    plot.hooks!.draw!.push(renderAnnotationListInTimeline);
  }

  $.plot.plugins.push({
    init,
    options: {},
    name: 'annotations',
    version: '1.0',
  });
})(jQuery);

function renderAnnotationListInTimeline(
  plot: IPlot,
  ctx: CanvasRenderingContext2D
) {
  const options: IFlotOptions = plot.getOptions();

  if (options.annotations?.length) {
    const plotOffset: { top: number; left: number } = plot.getPlotOffset();
    const extractedX = extractRange(plot, 'x');
    const extractedY = extractRange(plot, 'y');

    options.annotations.forEach((annotation: AnnotationType) => {
      const left: number =
        Math.floor(extractedX.axis.p2c(annotation.timestamp * 1000)) +
        plotOffset.left;

      renderAnnotationIcon({
        annotation,
        options,
        left,
      });

      drawAnnotationLine({
        ctx,
        yMin: plotOffset.top,
        yMax:
          Math.floor(extractedY.axis.p2c(extractedY.axis.min)) + plotOffset.top,
        left,
        color: annotation.color,
      });
    });
  }
}

function drawAnnotationLine({
  ctx,
  color,
  left,
  yMax,
  yMin,
}: {
  ctx: CanvasRenderingContext2D;
  color: Color;
  left: number;
  yMax: number;
  yMin: number;
}) {
  ctx.beginPath();
  ctx.strokeStyle = color.hex();
  ctx.lineWidth = 1;
  ctx.moveTo(left + 0.5, yMax);
  ctx.lineTo(left + 0.5, yMin);
  ctx.stroke();
}

function renderAnnotationIcon({
  annotation,
  options,
  left,
}: {
  annotation: AnnotationType;
  options: { wrapperId?: string };
  left: number;
}) {
  const annotationMarkElementId =
    `${options.wrapperId}_annotation_mark_`.concat(
      String(annotation.timestamp)
    );

  const annotationMarkElement = $(`#${annotationMarkElementId}`);

  if (!annotationMarkElement.length) {
    $(
      `<div id="${annotationMarkElementId}" style="position: absolute; top: 0; left: ${left}px; width: 0" />`
    ).appendTo(`#${options.wrapperId}`);
  } else {
    annotationMarkElement.css({ left });
  }

  const container = document.getElementById(annotationMarkElementId);
  if (!container) {
    throw new Error(
      'DOM state error: was not able to find container: ' +
        annotationMarkElementId
    );
  }
  const root = createRoot(container);
  root.render(
    <Provider store={store}>
      <AnnotationMark
        type={annotation.type}
        posX={left}
        color={annotation.color}
        value={{ content: annotation.content, timestamp: annotation.timestamp }}
      />
    </Provider>
  );
}
