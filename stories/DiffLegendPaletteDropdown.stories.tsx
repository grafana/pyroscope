import React from 'react';
import { ComponentMeta } from '@storybook/react';
import DiffPaletteDropdown from '../webapp/javascript/components/FlameGraph/FlameGraphComponent/DiffLegendPaletteDropdown';

export default {
  title: 'DiffPaletteDropdown',
} as ComponentMeta<typeof DiffPaletteDropdown>;

export const DefaultDiffPaletteDropdown = () => <DiffPaletteDropdown />;
