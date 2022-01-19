/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { ComponentMeta } from '@storybook/react';
import {
  DefaultPalette,
  ColorBlindPalette,
} from '../webapp/javascript/components/FlameGraph/FlameGraphComponent/colorPalette';
import DiffLegend from '../webapp/javascript/components/FlameGraph/FlameGraphComponent/DiffLegend';

export default {
  title: 'DiffLegend',
} as ComponentMeta<typeof DiffLegend>;

export const Default = () => <DiffLegend palette={DefaultPalette} />;
export const ColorBlind = () => <DiffLegend palette={ColorBlindPalette} />;
