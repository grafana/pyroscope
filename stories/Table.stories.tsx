/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Table from '../webapp/javascript/ui/Table';
import { randomId } from '../webapp/javascript/util/randomId';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import '../webapp/sass/profile.scss';

export default {
  title: 'Components/Table',
  component: Table,
} as ComponentMeta<typeof Table>;

const items = Array.from({ length: 20 }).map((a, i) => {
  return {
    id: i,
    value: randomId(),
  };
});

export const MyTable = () => {
  const headRow = [
    { name: '', label: 'Id', sortable: 1 },
    { name: '', label: 'Value', sortable: 1 },
  ];

  const bodyRows = items.map((a) => {
    return {
      onClick: () => alert(`clicked on ${JSON.stringify(a)}`),
      cells: [{ value: a.id }, { value: a.value }],
    };
  });

  return (
    <Table
      itemsPerPage={5}
      table={{
        type: 'filled',
        headRow,
        bodyRows,
      }}
    />
  );
};
