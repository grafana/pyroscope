/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import Icon from '@pyroscope/ui/Icon';
import { ComponentMeta } from '@storybook/react';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';
import '../sass/profile.scss';

export default {
  title: 'Components/Icon',
  component: Icon,
} as ComponentMeta<typeof Icon>;

export const BasicIcon = () => <Icon icon={faClock} />;
