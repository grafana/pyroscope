import React, { ReactNode } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faAmbulance } from '@fortawesome/free-solid-svg-icons/faAmbulance';

export const SPY_NAMES_ICONS: { [k: string]: ReactNode } = {
  rbspy: <FontAwesomeIcon icon={faAmbulance} />,
  'pyroscope-rs': <FontAwesomeIcon icon={faAmbulance} />,
  pyspy: <FontAwesomeIcon icon={faAmbulance} />,
  javaspy: <FontAwesomeIcon icon={faAmbulance} />,
  phpspy: <FontAwesomeIcon icon={faAmbulance} />,
  nodespy: <FontAwesomeIcon icon={faAmbulance} />,
  gospy: <FontAwesomeIcon icon={faAmbulance} />,
  dotnetspy: <FontAwesomeIcon icon={faAmbulance} />,
  ebpfspy: <FontAwesomeIcon icon={faAmbulance} />,
  unknown: <FontAwesomeIcon icon={faAmbulance} />,
};
