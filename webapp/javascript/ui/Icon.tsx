import React from 'react';
import {
  FontAwesomeIcon,
  FontAwesomeIconProps,
} from '@fortawesome/react-fontawesome';

export type IconProps = FontAwesomeIconProps;

// Icon is (currently) an indirect layer over FontAwesomeIcons
export default function Icon(props: IconProps) {
  const { icon } = props;
  return <FontAwesomeIcon icon={icon} />;
}
