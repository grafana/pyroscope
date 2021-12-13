/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Icon from '@ui/Icon';
import { ComponentMeta } from '@storybook/react';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';

export default {
  title: 'Components/Icon',
  component: Icon,
} as ComponentMeta<typeof Icon>;

export const BasicIcon = () => <Icon icon={faClock} />;
