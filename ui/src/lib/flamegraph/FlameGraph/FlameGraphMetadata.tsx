import { css } from '@emotion/css';
import { memo, type ReactNode } from 'react';

import { Icon, type IconType } from '@components/core/Icon';

import { formatShort } from '../format';

import { type ClickedItemData } from '../types';

import { type FlameGraphDataContainer } from './dataTransform';

type Props = {
  data: FlameGraphDataContainer;
  totalTicks: number;
  onFocusPillClick: () => void;
  onSandwichPillClick: () => void;
  focusedItem?: ClickedItemData;
  sandwichedLabel?: string;
};

const FlameGraphMetadata = memo(
  ({ data, focusedItem, totalTicks, sandwichedLabel, onFocusPillClick, onSandwichPillClick }: Props) => {
    const parts: ReactNode[] = [];
    const ticksVal = formatShort(totalTicks);

    const displayValue = data.valueDisplayProcessor(totalTicks);
    let unitValue = displayValue.text + displayValue.suffix;
    const unitTitle = data.getUnitTitle();
    if (unitTitle === 'Count') {
      if (!displayValue.suffix) {
        // Makes sure we don't show 123undefined or something like that if suffix isn't defined
        unitValue = displayValue.text;
      }
    }

    parts.push(
      <div className={styles.metadataPill} key={'default'}>
        {unitValue} | {ticksVal.text}
        {ticksVal.suffix} samples ({unitTitle})
      </div>
    );

    if (sandwichedLabel) {
      parts.push(
        <div key={'sandwich'} title={sandwichedLabel} className={styles.pillGroup}>
          <Icon size={12} name="angle-right" />
          <div className={styles.metadataPill}>
            <Icon size={12} name="sandwich" />
            <span className={styles.metadataPillName}>
              {sandwichedLabel.substring(sandwichedLabel.lastIndexOf('/') + 1)}
            </span>
            <PillCloseButton onClick={onSandwichPillClick} label="Remove sandwich view" />
          </div>
        </div>
      );
    }

    if (focusedItem) {
      const percentValue = totalTicks > 0 ? Math.round(10000 * (focusedItem.item.value / totalTicks)) / 100 : 0;
      const iconName: IconType = percentValue > 0 ? 'eye' : 'exclamation-circle';

      parts.push(
        <div key={'focus'} title={focusedItem.label} className={styles.pillGroup}>
          <Icon size={12} name="angle-right" />
          <div className={styles.metadataPill}>
            <Icon size={12} name={iconName} />
            &nbsp;{percentValue}% of total
            <PillCloseButton onClick={onFocusPillClick} label="Remove focus" />
          </div>
        </div>
      );
    }

    return <div className={styles.metadata}>{parts}</div>;
  }
);

FlameGraphMetadata.displayName = 'FlameGraphMetadata';

function PillCloseButton({ onClick, label }: { onClick: () => void; label: string }) {
  return (
    <button
      type="button"
      className={styles.pillCloseButton}
      onClick={onClick}
      aria-label={label}
      title={label}
    >
      <Icon name="times" size={12} />
    </button>
  );
}

const styles = {
  metadataPill: css({
    label: 'metadataPill',
    display: 'inline-flex',
    alignItems: 'center',
    gap: 4,
    background: 'var(--bg-secondary)',
    borderRadius: 'var(--radius-lg)',
    padding: '4px 8px',
    fontSize: 'var(--text-sm)',
    fontWeight: 'var(--weight-medium)',
    color: 'var(--text-secondary)',
  }),
  pillCloseButton: css({
    label: 'pillCloseButton',
    background: 'transparent',
    border: 'none',
    color: 'var(--text-secondary)',
    cursor: 'pointer',
    padding: '0 2px',
    display: 'inline-flex',
    alignItems: 'center',
    '&:hover': { color: 'var(--text-primary)' },
  }),
  pillGroup: css({
    display: 'inline-flex',
    alignItems: 'center',
  }),
  metadata: css({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 4,
    margin: '8px 0',
  }),
  metadataPillName: css({
    label: 'metadataPillName',
    maxWidth: '200px',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  }),
};

export default FlameGraphMetadata;
