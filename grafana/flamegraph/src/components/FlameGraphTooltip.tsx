import { css } from '@emotion/css';
import React, { LegacyRef } from 'react';

import { useStyles, Tooltip } from '@grafana/ui';

import { NAME_OFFSET } from '../constants';
import { TooltipData, SampleUnit } from './types';

type Props = {
  tooltipRef: LegacyRef<HTMLDivElement>,
  tooltipData: TooltipData,
  showTooltip: boolean,
};

const FlameGraphTooltip = ({tooltipRef, tooltipData, showTooltip}: Props) => {
  const styles = useStyles(getStyles);
  
  return (
    <div ref={tooltipRef} className={styles.tooltip}>
      {tooltipData &&
        <Tooltip 
          content={
            <div>
              <div className={styles.name}>{tooltipData.name}</div>
              <div>{tooltipData.percentTitle}: <b>{tooltipData.percentValue}%</b></div>
              <div>{tooltipData.unitTitle}: <b>{tooltipData.unitValue}</b></div>
              <div>Samples: <b>{tooltipData.samples}</b></div>
            </div>
          } 
          placement={'right'} 
          show={showTooltip}
        >
          <span></span>
        </Tooltip>
      }
    </div>
  )
}

export const getTooltipData = (profileTypeId: string, names: any, levels: any, totalTicks: number, levelIndex: any, barIndex: any): TooltipData => {
  let samples = levels[levelIndex][barIndex + 1];
  let percentTitle = '';
  let unitTitle = '';
  let unitValue = '';

  const sampleUnit = profileTypeId?.split(':').length === 5 ? profileTypeId.split(':')[2] : '';
  const name = `${names[levels[levelIndex][barIndex + NAME_OFFSET]]}`;
  const percent = Math.round(10000 * (samples / totalTicks)) / 100;

  switch (sampleUnit) {
    case SampleUnit.Bytes:
      unitValue = getUnitValue(
        samples, 
        [
          { divider: 1024, suffix: 'KB'},
          { divider: 1024, suffix: 'MB'},
          { divider: 1024, suffix: 'GB'},
          { divider: 1024, suffix: 'PT'},
        ],
      );
      percentTitle = '% of total RAM';
      unitTitle = 'RAM';
      break;

    case SampleUnit.Count:
      unitValue = getUnitValue(
        samples, 
        [
          { divider: 1000, suffix: 'K'},
          { divider: 1000, suffix: 'M'},
          { divider: 1000, suffix: 'G'},
          { divider: 1000, suffix: 'T'},
        ],
      );
      percentTitle = '% of total objects';
      unitTitle = 'Allocated objects';
      break;

    case SampleUnit.Nanoseconds:      
      unitValue = getUnitValue(
        // convert nanoseconds to seconds
        samples / 1000000000, 
        [
          { divider: 60, suffix: 'minutes'},
          { divider: 60, suffix: 'hours'},
          { divider: 24, suffix: 'days'},
        ],
        'seconds'
      );
      percentTitle = '% of total time';
      unitTitle = 'Time';
  }

  return {
    name: name,
    percentTitle: percentTitle,
    percentValue: percent,
    unitTitle: unitTitle,
    unitValue: unitValue,
    samples: samples.toLocaleString()
  }
};

const getUnitValue = (samples: number, units: any, fallbackSuffix = '') => {
  let unitValue: string;
  let suffix = '';

  for (let unit of units) {
    if (samples >= unit.divider) {
      suffix = unit.suffix;
      samples = samples / unit.divider;
    } else {
      break;
    }
  }

  unitValue = samples.toString();
  if (unitValue.toString().includes('.')) {
    const afterDot = unitValue.toString().split('.')[1];
    if (afterDot.length > 2) {
      unitValue = samples.toFixed(2);
    }
  }

  unitValue += ' ' + (suffix !== '' ? suffix : fallbackSuffix);

  return unitValue;
}

const getStyles = () => ({
  tooltip: css`
    position: fixed;
  `,
  name: css`
    margin-bottom: 10px;
  `,
});

export default FlameGraphTooltip;
