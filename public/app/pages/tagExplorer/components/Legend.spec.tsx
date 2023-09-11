import React from 'react';
import Color from 'color';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import type { TimelineGroupData } from '@pyroscope/components/TimelineChart/TimelineChartWrapper';
import type { Group } from '@pyroscope/legacy/models';

import Legend from './Legend';

const groups = [
  {
    tagName: 'tag1',
    color: Color('red'),
    data: {} as Group,
  },
  {
    tagName: 'tag2',
    color: Color('green'),
    data: {} as Group,
  },
];

describe('Component: Legend', () => {
  const renderLegend = (
    groups: TimelineGroupData[],
    handler: (v: string) => void
  ) => {
    render(
      <Legend
        activeGroup="All"
        groups={groups}
        handleGroupByTagValueChange={handler}
      />
    );
  };

  it('renders tags and colors correctly', () => {
    renderLegend(groups, () => {});

    expect(screen.getByTestId('legend')).toBeInTheDocument();
    expect(screen.getAllByTestId('legend-item')).toHaveLength(2);
    expect(screen.getAllByTestId('legend-item-color')).toHaveLength(2);
    screen.getAllByTestId('legend-item-color').forEach((element, index) => {
      expect(element).toHaveStyle(
        `background-color: ${groups[index].color.toString()}`
      );
    });
  });

  it('calls handleGroupByTagValueChange correctly', () => {
    const handleGroupByTagValueChangeMock = jest.fn();
    renderLegend(groups, handleGroupByTagValueChangeMock);

    expect(screen.getAllByTestId('legend-item')).toHaveLength(2);
    screen.getAllByTestId('legend-item').forEach((element) => {
      userEvent.click(element);

      expect(handleGroupByTagValueChangeMock).toHaveBeenCalledWith(
        element.textContent
      );
    });
  });
});
