import React, { useEffect, useState } from 'react';
import * as ReactDOM from 'react-dom';
import { PlotType, EventHolderType, EventType } from './types';
import { MenuItem, ControlledMenu, useMenuState } from '@szhsin/react-menu';

type ContextType = {
  init: (plot: PlotType) => void;
  options: unknown;
  name: string;
  version: string;
};

interface ContextMenuProps {
  x: number;
  y: number;
}

function MyElement(props: ContextMenuProps) {
  const [isOpen, setOpen] = useState(false);
  useEffect(() => {
    setOpen(true);
  }, []);

  const { x, y } = props;
  //  return <div>hey</div>;
  return (
    <ControlledMenu isOpen={isOpen} anchorPoint={{ x, y }}>
      <MenuItem key="focus">Item 1</MenuItem>
    </ControlledMenu>
  );
}

(function ($: JQueryStatic) {
  function init(this: ContextType, plot: PlotType) {
    const container = inject($);

    // TODO(eh-am): fix id
    $(`#timeline-chart-single`).bind('plotclick', (event, pos, item) => {
      console.log({
        range: Math.round(pos.x / 1000).toString(),
      });

      console.log({
        event,
      });
      ReactDOM.render(MyElement({ x: 0, y: 0 }), container?.[0]);
    });
  }

  ($ as ShamefulAny).plot.plugins.push({
    init,
    options: {},
    name: 'context_menu',
    version: '1.0',
  });
})(jQuery);

// TODO(eh-am)
const WRAPPER_ID = 'contextmenu_id';

const inject = ($: JQueryStatic) => {
  const parent = $(`#${WRAPPER_ID}`).length
    ? $(`#${WRAPPER_ID}`)
    : $(`<div id="${WRAPPER_ID}" />`);

  const par2 = $(`body`);

  return parent.appendTo(par2);
};
