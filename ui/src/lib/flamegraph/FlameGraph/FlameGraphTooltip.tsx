import { css } from '@emotion/css';
import { createPortal } from 'react-dom';

import { type CollapseConfig, type FlameGraphDataContainer, type LevelItem } from './dataTransform';

type Props = {
  data: FlameGraphDataContainer;
  totalTicks: number;
  position?: { x: number; y: number };
  item?: LevelItem;
  collapseConfig?: CollapseConfig;
};

const FlameGraphTooltip = ({ data, item, totalTicks, position, collapseConfig }: Props) => {
  if (!(item && position)) {
    return null;
  }

  const tooltipData = getTooltipData(data, item, totalTicks);

  return createPortal(
    <div
      className={styles.tooltipContainer}
      style={{ left: position.x + 15, top: position.y }}
      role="tooltip"
      aria-live="polite"
    >
      <div className={styles.tooltipContent}>
        <p className={styles.tooltipName}>
          {data.getLabel(item.itemIndexes[0])}
          {collapseConfig && collapseConfig.collapsed ? (
            <span>
              <br />
              and {collapseConfig.items.length} similar items
            </span>
          ) : (
            ''
          )}
        </p>
        <p className={styles.lastParagraph}>
          {tooltipData.unitTitle}
          <br />
          Total: <b>{tooltipData.unitValue}</b> ({tooltipData.percentValue}%)
          <br />
          Self: <b>{tooltipData.unitSelf}</b> ({tooltipData.percentSelf}%)
          <br />
          Samples: <b>{tooltipData.samples}</b>
        </p>
      </div>
    </div>,
    document.body
  );
};

type TooltipData = {
  percentValue: number;
  percentSelf: number;
  unitTitle: string;
  unitValue: string;
  unitSelf: string;
  samples: string;
};

export const getTooltipData = (data: FlameGraphDataContainer, item: LevelItem, totalTicks: number): TooltipData => {
  const displayValue = data.valueDisplayProcessor(item.value);
  const displaySelf = data.getSelfDisplay(item.itemIndexes);

  const percentValue = Math.round(10000 * (displayValue.numeric / totalTicks)) / 100;
  const percentSelf = Math.round(10000 * (displaySelf.numeric / totalTicks)) / 100;
  let unitValue = displayValue.text + displayValue.suffix;
  let unitSelf = displaySelf.text + displaySelf.suffix;

  const unitTitle = data.getUnitTitle();
  if (unitTitle === 'Count') {
    if (!displayValue.suffix) {
      // Makes sure we don't show 123undefined or something like that if suffix isn't defined
      unitValue = displayValue.text;
    }
    if (!displaySelf.suffix) {
      unitSelf = displaySelf.text;
    }
  }

  return {
    percentValue,
    percentSelf,
    unitTitle,
    unitValue,
    unitSelf,
    samples: displayValue.numeric.toLocaleString(),
  };
};

const styles = {
  tooltipContainer: css({
    label: 'tooltipContainer',
    position: 'fixed',
    pointerEvents: 'none',
    zIndex: 1000,
    overflow: 'hidden',
    background: 'var(--bg-elevated)',
    color: 'var(--text-primary)',
    border: '1px solid var(--border-medium)',
    borderRadius: 'var(--radius-md)',
    padding: '8px 12px',
    boxShadow: 'var(--shadow-md)',
    maxWidth: 400,
  }),
  tooltipContent: css({
    label: 'tooltipContent',
    fontSize: 'var(--text-sm)',
    width: '100%',
  }),
  tooltipName: css({
    label: 'tooltipName',
    marginTop: 0,
    wordBreak: 'break-all',
  }),
  lastParagraph: css({
    label: 'lastParagraph',
    marginBottom: 0,
  }),
};

export default FlameGraphTooltip;
