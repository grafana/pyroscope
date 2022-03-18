import React from 'react';
import { PanelProps } from '@grafana/data';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import { SimpleOptions } from './types';
// TODO: remove after FlamegraphRenderer is updated to typescript
// eslint-disable-next-line @typescript-eslint/no-explicit-any
import styles from './SimplePanel.module.css';

type Props = PanelProps<SimpleOptions>;

// eslint-disable-next-line import/prefer-default-export
export const SimplePanel: React.FC<Props> = ({ options, data }) => {
  // TODO
  // this can fail in so many ways
  // let's handle it better
  const flamebearer = (
    data.series[data.series.length - 1].fields[0].values as any
  ).buffer[0];

  return (
    <div className={`flamegraph-wrapper ${styles.panel}`}>
      <FlamegraphRenderer
        flamebearer={flamebearer}
        ExportData={<div />}
        onlyDisplay="flamegraph"
        showToolbar={options.showToolbar}
      />
    </div>
  );
};
