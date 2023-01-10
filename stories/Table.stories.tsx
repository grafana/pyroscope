/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Table, {
  useTableSort,
  BodyRow,
  TableBodyType,
} from '../webapp/javascript/ui/Table';
import { randomId } from '../webapp/javascript/util/randomId';
import { ComponentStory, ComponentMeta } from '@storybook/react';
import '../webapp/sass/profile.scss';

const Template: ComponentStory<typeof Table> = (args) => <Table {...args} />;

export default {
  title: 'Components/Table',
  component: Table,
} as ComponentMeta<typeof Table>;

const items = Array.from({ length: 20 }).map(() => {
  return {
    id: randomId(),
    value: Math.random(),
  };
});

export const MyTable = () => {
  const headRow = [
    { name: '', label: 'Id', sortable: 0 },
    { name: '', label: 'Value', sortable: 0 },
  ];

  const bodyRows = items.map((a) => {
    return {
      onClick: () => alert(`clicked on ${JSON.stringify(a)}`),
      cells: [{ value: a.id }, { value: a.value }],
    };
  });

  return (
    <Table
      table={{
        type: 'filled',
        headRow,
        bodyRows,
      }}
    />
  );
};
