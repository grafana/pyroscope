import React from 'react';
import * as ReactDOM from 'react-dom';
import { randomId } from '@webapp/util/randomId';

const WRAPPER_ID = randomId('context_menu');

export interface ContextMenuProps {
  click: {
    /** The X position in the window where the click originated */
    pageX: number;
    /** The Y position in the window where the click originated */
    pageY: number;
  };
  timestamp: number;
  containerEl: HTMLElement;
}

(function ($: JQueryStatic) {
  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    const placeholder = plot.getPlaceholder();

    function onClick(
      event: unknown,
      pos: { x: number; pageX: number; pageY: number }
    ) {
      const options: jquery.flot.plotOptions & {
        ContextMenu?: React.FC<ContextMenuProps>;
      } = plot.getOptions();
      const container = inject($);
      const containerEl = container?.[0];

      // unmount any previous menus
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

    if (plot.hooks?.bindEvents) {
      plot.hooks.bindEvents.push(function () {
        placeholder.bind('plotclick', onClick);
      });
    }

    if (plot.hooks?.shutdown) {
      plot.hooks.shutdown.push(function () {
        placeholder.unbind('plotclick', onClick);

        const container = inject($);

        ReactDOM.unmountComponentAtNode(container?.[0]);
      });
    }
  }

  $.plot.plugins.push({
    init,
    options: {},
    name: 'context_menu',
    version: '1.0',
  });
})(jQuery);

function inject($: JQueryStatic) {
  const alreadyInitialized = $(`#${WRAPPER_ID}`).length > 0;

  if (alreadyInitialized) {
    return $(`#${WRAPPER_ID}`);
  }

  const body = $('body');
  return $(`<div id="${WRAPPER_ID}" />`).appendTo(body);
}
