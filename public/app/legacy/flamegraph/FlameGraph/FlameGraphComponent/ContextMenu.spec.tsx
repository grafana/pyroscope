/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { act, render, screen } from '@testing-library/react';
import { MenuItem } from '@pyroscope/ui/Menu';
import userEvent from '@testing-library/user-event';

import ContextMenu, { ContextMenuProps } from './ContextMenu';

const { queryByRole, queryAllByRole, getByRole } = screen;

function TestCanvas(props: Omit<ContextMenuProps, 'canvasRef'>) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null);

  return (
    <>
      <canvas data-testid="canvas" ref={canvasRef} />
      <ContextMenu data-testid="contextmenu" canvasRef={canvasRef} {...props} />
    </>
  );
}

describe('ContextMenu', () => {
  it('works', () => {
    let hasBeenClicked = false;

    const xyToMenuItems = () => {
      return [
        <MenuItem
          key="test"
          onClick={() => {
            hasBeenClicked = true;
          }}
        >
          Test
        </MenuItem>,
      ];
    };

    render(
      <TestCanvas
        xyToMenuItems={xyToMenuItems}
        onClose={() => {}}
        onOpen={() => {}}
      />
    );

    expect(queryByRole('menu')).not.toBeInTheDocument();

    // trigger a right click
    act(() => userEvent.click(screen.getByTestId('canvas'), { buttons: 2 }));

    expect(queryByRole('menu')).toBeVisible();
    expect(queryAllByRole('menuitem')).toHaveLength(1);

    act(() => userEvent.click(getByRole('menuitem')));
    expect(hasBeenClicked).toBe(true);
  });

  it('shows different items depending on the clicked node', () => {
    const xyToMenuItems = jest.fn();

    render(
      <TestCanvas
        xyToMenuItems={xyToMenuItems}
        onClose={() => {}}
        onOpen={() => {}}
      />
    );

    expect(queryByRole('menu')).not.toBeInTheDocument();

    // trigger a right click
    xyToMenuItems.mockReturnValueOnce([<MenuItem key="1">1</MenuItem>]);
    act(() => userEvent.click(screen.getByTestId('canvas'), { buttons: 2 }));
    expect(queryAllByRole('menuitem')).toHaveLength(1);

    xyToMenuItems.mockReturnValueOnce([
      <MenuItem key="1">1</MenuItem>,
      <MenuItem key="2">2</MenuItem>,
    ]);
    act(() => userEvent.click(screen.getByTestId('canvas'), { buttons: 2 }));
    expect(queryAllByRole('menuitem')).toHaveLength(2);
  });
});
