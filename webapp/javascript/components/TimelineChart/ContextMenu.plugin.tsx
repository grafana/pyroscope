import React from 'react';
import * as ReactDOM from 'react-dom';
import store from '@webapp/redux/store';
import { Provider } from 'react-redux';
import { PlotType } from './types';

type ContextType = {
  init: (plot: PlotType) => void;
  options: unknown;
  name: string;
  version: string;
  contextMenu?: React.ReactNode;
};

(function ($: JQueryStatic) {
  function init(this: ContextType, plot: PlotType) {
    const container = inject($);
    const containerEl = container?.[0];
    const options = plot.getOptions();

    // TODO(eh-am): fix id
    $(`#timeline-chart-single`).bind('plotclick', (event, pos, item) => {
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

  ($ as ShamefulAny).plot.plugins.push({
    init,
    options: {},
    name: 'context_menu',
    version: '1.0',
  });
})(jQuery);

// TODO(eh-am): we may have multiple context menus
const WRAPPER_ID = 'contextmenu_id';

const inject = ($: JQueryStatic) => {
  const parent = $(`#${WRAPPER_ID}`).length
    ? $(`#${WRAPPER_ID}`)
    : $(`<div id="${WRAPPER_ID}" />`);

  const par2 = $(`body`);

  return parent.appendTo(par2);
};
