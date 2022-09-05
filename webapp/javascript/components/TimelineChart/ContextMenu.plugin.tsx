import React, { useEffect, useState } from 'react';
import * as ReactDOM from 'react-dom';
import { useAppDispatch } from '@webapp/redux/hooks';
import {
  addAnnotation,
  fetchSingleView,
} from '@webapp/redux/reducers/continuous';
import { PlotType, EventHolderType, EventType } from './types';
import {
  Menu,
  MenuItem,
  ControlledMenu,
  useMenuState,
} from '@szhsin/react-menu';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';
import Popover, {
  PopoverHeader,
  PopoverBody,
  PopoverFooter,
} from '@webapp/ui/Popover';
import Button from '@webapp/ui/Button';
import InputField from '@webapp/ui/InputField';
import store, { persistor } from '@webapp/redux/store';
import { Provider } from 'react-redux';

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
  const dispatch = useAppDispatch();

  useEffect(() => {
    setOpen(true);
  }, []);

  const [isModalOpen, setModalOpen] = useState(false);
  const handleOutsideClick = () => setModalOpen(false);

  // TODO(eh-am): handle out of bounds positioning
  const popoverPosition = {
    left: `${x}px`,
    top: `${y}px`,
    position: 'absolute' as const,
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
      <div style={popoverPosition}>
        <Popover isModalOpen={isModalOpen} setModalOpenStatus={setModalOpen}>
          <PopoverHeader>Add annotation</PopoverHeader>
          <PopoverBody>
            <form
              id="annotation-form"
              name="annotation-form"
              onSubmit={async (event) => {
                event.preventDefault();

                const newAnnotation = {
                  appName: 'myapp',
                  content: event.target.content.value,
                  timestamp,
                };
                await dispatch(addAnnotation(newAnnotation));
                await dispatch(fetchSingleView(null));
              }}
            >
              <InputField
                type="text"
                label="Text"
                placeholder=""
                name="content"
                required
              />
            </form>
          </PopoverBody>
          <PopoverFooter>
            <Button type="submit" kind="secondary" form="annotation-form">
              Save
            </Button>
          </PopoverFooter>
        </Popover>
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
        <Provider store={store}>
          <MyElement x={pos.pageX} y={pos.pageY} timestamp={timestamp} />
        </Provider>,
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
