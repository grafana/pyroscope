import { css } from '@emotion/css';

import { type GrafanaTheme2 } from '@grafana/data';
import { Portal, useStyles2, VizTooltipContainer } from '@grafana/ui';

import { type CollapseConfig, type FlameGraphDataContainer, type LevelItem } from './dataTransform';

type Props = {
  data: FlameGraphDataContainer;
  totalTicks: number;
  position?: { x: number; y: number };
  item?: LevelItem;
  collapseConfig?: CollapseConfig;
};

const FlameGraphTooltip = ({ data, item, totalTicks, position, collapseConfig }: Props) => {
  const styles = useStyles2(getStyles);

  if (!(item && position)) {
    return null;
  }

  const tooltipData = getTooltipData(data, item, totalTicks);

  return (
    <Portal>
      <VizTooltipContainer className={styles.tooltipContainer} position={position} offset={{ x: 15, y: 0 }}>
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
      </VizTooltipContainer>
    </Portal>
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
      // Makes sure we don't show 123undefined or something like that if suffix isn't defined
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

const getStyles = (theme: GrafanaTheme2) => ({
  tooltipContainer: css({
    title: 'tooltipContainer',
    overflow: 'hidden',
  }),
  tooltipContent: css({
    title: 'tooltipContent',
    fontSize: theme.typography.bodySmall.fontSize,
    width: '100%',
  }),
  tooltipName: css({
    title: 'tooltipName',
    marginTop: 0,
    wordBreak: 'break-all',
  }),
  lastParagraph: css({
    title: 'lastParagraph',
    marginBottom: 0,
  }),
  name: css({
    title: 'name',
    marginBottom: '10px',
  }),
});

export default FlameGraphTooltip;
