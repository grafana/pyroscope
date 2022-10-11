import React from 'react';
import * as ReactDOM from 'react-dom';
import Color from 'color';
import { Provider } from 'react-redux';
import store from '@webapp/redux/store';
import { randomId } from '@webapp/util/randomId';
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
  ContextMenu?: React.FC<ContextMenuProps>;
  wrapperId?: string;
}

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

const WRAPPER_ID = randomId('annotations');

(function ($) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const placeholder = plot.getPlaceholder();

    function onClick(
      _: unknown,
      pos: { x: number; pageX: number; pageY: number; y: number }
    ) {
      const options: IFlotOptions = plot.getOptions();
      const container = inject($);
      const containerEl = container?.[0];

      ReactDOM.unmountComponentAtNode(containerEl);

      const ContextMenu = options?.ContextMenu;

      if (ContextMenu && containerEl) {
        ReactDOM.render(
          <ContextMenu
            click={{ ...pos }}
            containerEl={containerEl}
            timestamp={Math.round(pos.x / 1000)}
          />,
          containerEl
        );
      }
    }

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

    plot.hooks!.bindEvents!.push(() => {
      placeholder.bind('plotclick', onClick);
    });

    plot.hooks!.shutdown!.push(() => {
      placeholder.unbind('plotclick', onClick);

      const container = inject($);

      ReactDOM.unmountComponentAtNode(container?.[0]);
    });
  }

  $.plot.plugins.push({
    init,
    options: {},
    name: 'annotations',
    version: '1.0',
  });
})(jQuery);

const inject = ($: JQueryStatic) => {
  const alreadyInitialized = $(`#${WRAPPER_ID}`).length > 0;

  if (alreadyInitialized) {
    return $(`#${WRAPPER_ID}`);
  }

  const body = $('body');
  return $(`<div id="${WRAPPER_ID}" />`).appendTo(body);
};

const drawAnnotationLine = ({
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
}) => {
  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.lineWidth = 1;
  ctx.moveTo(left + 0.5, yMax);
  ctx.lineTo(left + 0.5, yMin);
  ctx.stroke();
};

const renderAnnotationMark = ({
  annotation,
  options,
  left,
}: {
  annotation: AnnotationType;
  options: { wrapperId?: string };
  left: number;
}) => {
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
};
