import React from 'react';
import * as ReactDOM from 'react-dom';
import store from '@webapp/redux/store';
import { Provider } from 'react-redux';
import { randomId } from '@webapp/util/randomId';
import { PlotType } from './types';

const WRAPPER_ID = randomId('contextMenu');

type ContextType = {
  init: (plot: PlotType) => void;
  options: unknown;
  name: string;
  version: string;
  contextMenu?: React.ReactNode;
};

(function ($: JQueryStatic) {
  function init(this: ContextType, plot: PlotType) {
    // The element we will add the contextMenu
    const container = inject($);
    const containerEl = container?.[0];
    // The flotjs wrapper
    const flotEl = plot.getPlaceholder();

    const options = plot.getOptions();

    // TODO(eh-am): flot only supports plotclick (left-click)
    // to support right-click we need to implement it ourselves
    $(flotEl[0]).bind('plotclick', (event, pos, item) => {
      // TODO(eh-am): why do we need this conversion?
      const timestamp = Math.round(pos.x / 1000);
      const { ContextMenu } = options;

      // unmount any previous menus
      ReactDOM.unmountComponentAtNode(containerEl);

      // TODO(eh-am): use portal instead of wrapping of sharing the same store?
      // https://stackoverflow.com/questions/52660770/how-to-communicate-reactdom-render-with-other-reactdom-render
      if (ContextMenu) {
        ReactDOM.render(
          <Provider store={store}>
            <ContextMenu x={pos.pageX} y={pos.pageY} timestamp={timestamp} />
          </Provider>,
          containerEl
        );
      }
    });
  }

  // TODO(eh-am): add type
  ($ as ShamefulAny).plot.plugins.push({
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
