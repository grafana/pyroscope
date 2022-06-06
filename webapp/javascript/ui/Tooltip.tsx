/* eslint-disable jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions */
import React from 'react';
import styles from './Tooltip.module.scss';

const Tooltip = ({
  syncEnabled,
  visible,
}: {
  syncEnabled: string | boolean;
  visible: boolean;
}) => {
  return (
    <div
      onClick={(e) => e.stopPropagation()}
      className={`${visible ? styles.visible : ''} ${
        syncEnabled ? styles.tooltipSyncEnabled : styles.tooltip
      }`}
      role="tooltip"
    >
      {syncEnabled ? 'Unsync search bars' : 'Sync search bars'}
    </div>
  );
};

export default Tooltip;
