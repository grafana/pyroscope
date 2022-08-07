/* eslint-disable react/jsx-props-no-spreading */
import React, { useRef } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Units } from '@pyroscope/models/src';

import TableTooltip, { TableTooltipProps } from './TableTooltip';

function TestTable(props: Omit<TableTooltipProps, 'tableBodyRef'>) {
  const tableBodyRef = useRef<HTMLTableSectionElement>(null);

  return (
    <>
      <table>
        <tbody data-testid="table-body" ref={tableBodyRef} />
      </table>
      <TableTooltip
        {...(props as TableTooltipProps)}
        tableBodyRef={tableBodyRef}
      />
    </>
  );
}

describe('TableTooltip', () => {
  const renderTable = (
    props: Omit<TableTooltipProps, 'tableBodyRef' | 'palette'>
  ) => render(<TestTable {...props} />);

  it('should render TableTooltip', () => {
    const props = {
      numTicks: 100,
      sampleRate: 100,
      units: 'samples' as Units,
    };

    renderTable(props);

    userEvent.hover(screen.getByTestId('table-body'));

    expect(screen.getByTestId('tooltip')).toBeInTheDocument();
  });
});
