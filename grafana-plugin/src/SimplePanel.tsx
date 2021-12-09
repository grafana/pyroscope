import React from 'react';
import { PanelProps } from '@grafana/data';
import { stylesFactory, useTheme } from '@grafana/ui';
import { Option } from 'prelude-ts';
import { FitModes } from '../../webapp/javascript/util/fitMode';
import { SimpleOptions } from './types';
import Flamegraph from '../../webapp/javascript/components/FlameGraph/FlameGraphComponent/index';
import FlamegraphRenderer from '../../webapp/javascript/components/FlameGraph/FlameGraphRenderer';
import styles from './SimplePanel.module.css';

type Props = PanelProps<SimpleOptions>;

// eslint-disable-next-line import/prefer-default-export
export const SimplePanel: React.FC<Props> = ({
  options,
  data,
  width,
  height,
}) => {
  const theme = useTheme();

  // TODO
  // this can fail in so many ways
  // let's handle it better
  const flamebearer = (
    data.series[data.series.length - 1].fields[0].values as any
  ).buffer[0];

  return (
    <>
      <div className={`flamegraph-wrapper ${styles.panel}`}>
        <FlamegraphRenderer flamebearer={flamebearer} viewType="grafana" />
      </div>
    </>
  );
};
