import React from 'react';
import * as ReactDOM from 'react-dom';
import { randomId } from '@webapp/util/randomId';
import { Provider } from 'react-redux';
import store from '@webapp/redux/store';

// Pre calculated once
// TODO: does this work with multiple contextMenus?
const WRAPPER_ID = randomId('contextMenu');

export interface ContextMenuProps {
  click: {
    /** The X position in the window where the click originated */
    pageX: number;
    /** The X position in the window where the click originated */
    pageY: number;
  };
  timestamp: number;
}

(function ($: JQueryStatic) {
  let globalPlot: jquery.flot.plot;

  function onClick(
    event: unknown,
    pos: { x: number; pageX: number; pageY: number }
  ) {
    // TODO: precalculate these somehow?
    const container = inject($);
    const containerEl = container?.[0];

    // TODO(eh-am): why do we need this conversion?
    const timestamp = Math.round(pos.x / 1000);

    // TODO(eh-am): improve typing
    const ContextMenu = (globalPlot.getOptions() as ShamefulAny)
      .ContextMenu as React.FC<ContextMenuProps>;

    // unmount any previous menus
    ReactDOM.unmountComponentAtNode(containerEl);

    // TODO(eh-am): use portal instead of wrapping of sharing the same store?
    // https://stackoverflow.com/questions/52660770/how-to-communicate-reactdom-render-with-other-reactdom-render
    if (ContextMenu) {
      // TODO(eh-am): we need to add a Provider
      ReactDOM.render(
        <Provider store={store}>
          <ContextMenu click={{ ...pos }} timestamp={timestamp} />
        </Provider>,
        containerEl
      );
    }
  }

  function init(plot: jquery.flot.plot & jquery.flot.plotOptions) {
    // update closure
    globalPlot = plot;

    const flotEl = plot.getPlaceholder();

    // Register events and shutdown
    // It's important to bind/unbind to the SAME element
    // Since a plugin may be register/unregistered multiple times due to react re-rendering

    // TODO: not entirely sure when these are disabled
    if (plot.hooks?.bindEvents) {
      plot.hooks.bindEvents.push(function () {
        flotEl.bind('plotclick', onClick);
      });
    }

    if (plot.hooks?.shutdown) {
      plot.hooks.shutdown.push(function () {
        flotEl.unbind('plotclick', onClick);
        // TODO(eh-am): get rid of contextMenu wrapper in the DOM?
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

const inject = ($: JQueryStatic) => {
  const alreadyInitialized = $(`#${WRAPPER_ID}`).length > 0;

  if (alreadyInitialized) {
    return $(`#${WRAPPER_ID}`);
  }

  const body = $('body');
  return $(`<div id="${WRAPPER_ID}" />`).appendTo(body);
};
