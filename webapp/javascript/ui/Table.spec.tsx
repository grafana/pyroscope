import React from 'react';
import { renderHook, act } from '@testing-library/react-hooks';
import { render, within, screen } from '@testing-library/react';
import Table, { useTableSort } from './Table';

const mockHeadRow = [
  { name: 'self', label: 'test col2', sortable: 1 },
  { name: 'name', label: 'test col1', sortable: 1 },
  { name: 'total', label: 'test col3', sortable: 1 },
  { name: 'selfLeft', label: 'test col4', sortable: 1 },
  { name: 'selfRight', label: 'test col5', sortable: 1 },
  { name: 'selfDiff', label: 'test col6', sortable: 1 },
  { name: 'totalLeft', label: 'test col7', sortable: 1 },
  { name: 'totalRight', label: 'test col8', sortable: 1 },
  { name: 'totalDiff', label: 'test col9', sortable: 1 },
];

describe('Hook: useTableSort', () => {
  const render = () => renderHook(() => useTableSort(mockHeadRow)).result;

  it('should return initial sort values', () => {
    const hook = render();
    expect(hook.current).toStrictEqual({
      sortBy: 'self',
      sortByDirection: 'desc',
      updateSortParams: expect.any(Function),
    });
  });

  it('should update sort direction', () => {
    const hook = render();

    expect(hook.current.sortByDirection).toBe('desc');
    act(() => {
      hook.current.updateSortParams('self');
    });
    expect(hook.current.sortByDirection).toBe('asc');
  });

  it('should update sort value and sort direction', () => {
    const hook = render();

    expect(hook.current).toMatchObject({
      sortBy: 'self',
      sortByDirection: 'desc',
    });

    act(() => {
      hook.current.updateSortParams('name');
    });
    expect(hook.current).toMatchObject({
      sortBy: 'name',
      sortByDirection: 'desc',
    });

    act(() => {
      hook.current.updateSortParams('name');
    });
    expect(hook.current).toMatchObject({
      sortBy: 'name',
      sortByDirection: 'asc',
    });
  });
});

describe.only('pagination', () => {
  const header = [{ name: 'id', label: 'Id' }];
  const rows = Array.from({ length: 10 }).map((item, index) => {
    return {
      cells: [{ value: index }],
    };
  });

  it('does not paginate by default', async () => {
    render(
      <Table table={{ type: 'filled', headRow: header, bodyRows: rows }} />
    );

    const tbody = document.getElementsByTagName('tbody')[0];
    const items = await within(tbody).findAllByRole('row');
    expect(items).toHaveLength(10);
  });

  it('paginates', async () => {
    render(
      <Table
        itemsPerPage={1}
        table={{
          type: 'filled',
          headRow: header,
          bodyRows: rows,
        }}
      />
    );

    const tbody = document.getElementsByTagName('tbody')[0];
    const items = await within(tbody).findAllByRole('row');
    expect(items).toHaveLength(1);
    expect(items[0]).toHaveTextContent('0');
  });
});
