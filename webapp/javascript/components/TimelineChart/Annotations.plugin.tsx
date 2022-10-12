import React from 'react';
import * as ReactDOM from 'react-dom';
import Color from 'color';
import { Provider } from 'react-redux';
import store from '@webapp/redux/store';
import { CtxType } from './types';
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

(function ($) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    plot.hooks!.draw!.push((plot, ctx: CtxType) => {
      const options: IFlotOptions = plot.getOptions();

      if (options.annotations?.length) {
        const plotOffset: { top: number; left: number } = plot.getPlotOffset();
        const extractedX = extractRange(plot, 'x');
        const extractedY = extractRange(plot, 'y');

        options.annotations.forEach((a: AnnotationType) => {
          const left: number =
            Math.floor(extractedX.axis.p2c(a.timestamp * 1000)) +
            plotOffset.left;

          renderAnnotationMark({
            annotation: a,
            options,
            left,
          });

          drawAnnotationLine({
            ctx,
            yMin: plotOffset.top,
            yMax:
              Math.floor(extractedY.axis.p2c(extractedY.axis.min)) +
              plotOffset.top,
            left,
            color: a.color,
          });
        });
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

function drawAnnotationLine({
  ctx,
  color,
  left,
  yMax,
  yMin,
}: {
  ctx: CtxType;
  color: Color;
  left: number;
  yMax: number;
  yMin: number;
}) {
  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.lineWidth = 1;
  ctx.moveTo(left + 0.5, yMax);
  ctx.lineTo(left + 0.5, yMin);
  ctx.stroke();
}

function renderAnnotationMark({
  annotation,
  options,
  left,
}: {
  annotation: AnnotationType;
  options: { wrapperId?: string };
  left: number;
}) {
  const annotationMarkElementId = 'annotation_mark_'.concat(
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

  ReactDOM.render(
    <Provider store={store}>
      <AnnotationMark
        type={annotation.type}
        color={annotation.color}
        value={{ content: annotation.content, timestamp: annotation.timestamp }}
      />
    </Provider>,
    document.getElementById(annotationMarkElementId)
  );
}
