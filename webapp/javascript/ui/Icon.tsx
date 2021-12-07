import React from 'react';
import {
  FontAwesomeIcon,
  FontAwesomeIconProps,
} from '@fortawesome/react-fontawesome';

// Icon is (currently) an indirect layer over FontAwesomeIcons
export default function Icon(props: FontAwesomeIconProps) {
  const { icon } = props;
  return <FontAwesomeIcon icon={icon} />;
}
