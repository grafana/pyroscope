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

const inject = ($: JQueryStatic) => {
  const alreadyInitialized = $(`#${WRAPPER_ID}`).length > 0;

  if (alreadyInitialized) {
    return $(`#${WRAPPER_ID}`);
  }

  const body = $('body');
  return $(`<div id="${WRAPPER_ID}" />`).appendTo(body);
};

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
      const o: IFlotOptions = plot.getOptions();

      if (o.annotations?.length) {
        const plotOffset: { top: number; left: number } = plot.getPlotOffset();
        const extractedX = extractRange(plot, 'x');
        const extractedY = extractRange(plot, 'y');

        o.annotations.forEach((a: AnnotationType) => {
          const left: number =
            Math.floor(extractedX.axis.p2c(a.timestamp * 1000)) +
            plotOffset.left;

          const annotationMarkElementId = 'annotation_mark_'.concat(
            String(a.timestamp)
          );

          const annotationMarkElement = $(`#${annotationMarkElementId}`);

          if (!annotationMarkElement.length) {
            $(
              `<div id="${annotationMarkElementId}" style="position: absolute; top: 0; left: ${left}px; width: 0" />`
            ).appendTo(`#${o.wrapperId}`);
          } else {
            annotationMarkElement.css({ left });
          }

          ReactDOM.render(
            <Provider store={store}>
              <AnnotationMark
                type={a.type}
                color={a.color}
                value={{ content: a.content, timestamp: a.timestamp }}
              />
            </Provider>,
            document.getElementById(annotationMarkElementId)
          );

          const yMax =
            Math.floor(extractedY.axis.p2c(extractedY.axis.min)) +
            plotOffset.top;
          const yMin = 0 + plotOffset.top;
          const lineWidth = 1;
          const subPixel = lineWidth / 2 || 0;

          // draw vertical line
          ctx.beginPath();
          ctx.strokeStyle = a.color;
          ctx.lineWidth = lineWidth;
          ctx.moveTo(left + subPixel, yMax);
          ctx.lineTo(left + subPixel, yMin);
          ctx.stroke();
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
