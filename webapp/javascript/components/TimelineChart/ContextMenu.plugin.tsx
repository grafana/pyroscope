import React from 'react';
import * as ReactDOM from 'react-dom';
import { randomId } from '@webapp/util/randomId';
import { Provider } from 'react-redux';
import store from '@webapp/redux/store';

// Pre calculated once
// TODO(eh-am): does this work with multiple contextMenus?
const WRAPPER_ID = randomId('contextMenu');

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
    function onClick(
      event: unknown,
      pos: { x: number; pageX: number; pageY: number }
    ) {
      const container = inject($);
      const containerEl = container?.[0];

      // unmount any previous menus
      ReactDOM.unmountComponentAtNode(containerEl);

      // TODO(eh-am): improve typing
      const ContextMenu = (plot.getOptions() as ShamefulAny).ContextMenu as
        | React.FC<ContextMenuProps>
        | undefined;

      if (ContextMenu && containerEl) {
        // TODO(eh-am): why do we need this conversion?
        const timestamp = Math.round(pos.x / 1000);

        // Add a Provider (reux) so that we can communicate with the main app via actions
        // idea from https://stackoverflow.com/questions/52660770/how-to-communicate-reactdom-render-with-other-reactdom-render
        // TODO(eh-am): add a global Context too?
        ReactDOM.render(
          <Provider store={store}>
            <ContextMenu
              click={{ ...pos }}
              containerEl={containerEl}
              timestamp={timestamp}
            />
          </Provider>,
          containerEl
        );
      }
    }

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

        const container = inject($);
        const containerEl = container?.[0];

        // unmount any previous menus
        ReactDOM.unmountComponentAtNode(containerEl);
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
