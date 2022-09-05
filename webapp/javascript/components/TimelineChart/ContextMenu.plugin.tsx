import React, { useEffect, useState } from 'react';
import * as ReactDOM from 'react-dom';
import { PlotType, EventHolderType, EventType } from './types';
import {
  Menu,
  MenuItem,
  ControlledMenu,
  useMenuState,
} from '@szhsin/react-menu';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';

type ContextType = {
  init: (plot: PlotType) => void;
  options: unknown;
  name: string;
  version: string;
};

interface ContextMenuProps {
  x: number;
  y: number;

  /** the timestamp of the clicked item */
  timestamp: number;
}

function MyElement(props: ContextMenuProps) {
  const { x, y, timestamp } = props;
  const [isOpen, setOpen] = useState(false);

  useEffect(() => {
    setOpen(true);
  }, []);

  const [isModalOpen, setModalOpen] = useState(false);
  const handleOutsideClick = () => setModalOpen(false);

  const modalProps = {
    isModalOpen: isOpen,
    setModalOpenStatus: setModalOpen,
    handleOutsideClick,
    toggleText: 'toggle text',
    headerEl: 'header element',
    leftSideEl: (
      <ul>
        <li>1</li>
        <li>2</li>
      </ul>
    ),
    rightSideEl: (
      <ul>
        <li>3</li>
        <li>4</li>
      </ul>
    ),
    footerEl: 'footer element or string',
  };
  return (
    <>
      <ControlledMenu
        isOpen={isOpen}
        anchorPoint={{ x, y }}
        onClose={() => setOpen(false)}
      >
        <MenuItem key="focus" onClick={() => setModalOpen(true)}>
          Add annotation
        </MenuItem>
      </ControlledMenu>
      <div id="my-modal">
        <ModalWithToggle {...modalProps} />
      </div>
    </>
  );
}

(function ($: JQueryStatic) {
  function init(this: ContextType, plot: PlotType) {
    const container = inject($);
    const containerEl = container?.[0];

    // TODO(eh-am): fix id
    $(`#timeline-chart-single`).bind('plotclick', (event, pos, item) => {
      const timestamp = Math.round(pos.x / 1000);

      // unmount any previous menus
      ReactDOM.unmountComponentAtNode(containerEl);

      ReactDOM.render(
        <MyElement x={pos.pageX} y={pos.pageY} timestamp={timestamp} />,
        containerEl
      );
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
