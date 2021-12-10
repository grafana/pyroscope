import React from 'react';
import { PanelProps } from '@grafana/data';
import { SimpleOptions } from './types';
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
  //  const theme = useTheme();

  console.log('options', { options });
  // TODO
  // this can fail in so many ways
  // let's handle it better
  const flamebearer = (
    data.series[data.series.length - 1].fields[0].values as any
  ).buffer[0];

  console.log('width', width);

  return (
    <>
      <div className={`flamegraph-wrapper ${styles.panel}`}>
        <FlamegraphRenderer
          flamebearer={flamebearer}
          display="flamegraph"
          viewType="single"
          showToolbar={options.showToolbar}
        />
      </div>
    </>
  );
};
