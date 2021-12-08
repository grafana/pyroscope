// @ts-nocheck
import React from 'react';
import { PanelProps } from '@grafana/data';
import { SimpleOptions } from 'types';
import { css } from 'emotion';
import { stylesFactory, useTheme } from '@grafana/ui';
import { Option } from 'prelude-ts';
import Flamegraph from '../../webapp/javascript/components/FlameGraph/FlameGraphComponent/index';
import { Units } from '@utils/format';

interface Props extends PanelProps<SimpleOptions> {}

const simpleTree = {
  topLevel: 0,
  rangeMin: 0,
  format: 'single' as const,
  numTicks: 988,
  sampleRate: 100,
  names: [
    'total',
    'runtime.main',
    'main.slowFunction',
    'main.work',
    'main.main',
    'main.fastFunction',
  ],
  levels: [
    [0, 988, 0, 0],
    [0, 988, 0, 1],
    [0, 214, 0, 5, 214, 3, 2, 4, 217, 771, 0, 2],
    [0, 214, 214, 3, 216, 1, 1, 5, 217, 771, 771, 3],
  ],

  rangeMax: 1,
  units: Units.Samples,
  fitMode: 'HEAD',

  spyName: 'gospy',
};

function noopExportData() {
  return <div></div>;
}

export const SimplePanel: React.FC<Props> = ({
  options,
  data,
  width,
  height,
}) => {
  const theme = useTheme();
  const styles = getStyles();

  // TODO
  // this can fail in so many ways
  // let's handle it better
  const flamebearer =
    data.series[data.series.length - 1].fields[0].values.buffer[0];

  return (
    <>
      <div className={styles.app}>
        <div className={`${styles.appContainer} flamegraph-wrapper`}>
          <Flamegraph
            flamebearer={flamebearer}
            zoom={Option.none()}
            focusedNode={Option.none()}
            highlightQuery=""
            onZoom={() => {}}
            onFocusOnNode={() => {}}
            onReset={() => {}}
            isDirty={() => false}
            ExportData={noopExportData}
          />
        </div>
      </div>
    </>
  );
};

const getStyles = stylesFactory(() => {
  return {
    //    app: css`
    //      height: 100%;
    //      min-height: 100%;
    //      display: flex;
    //      flex-direction: column;
    //      .flamegraph-tooltip {
    //        position: fixed;
    //      }
    //    `,
    //    appContainer: css`
    //      flex: 1 0 auto;
    //      position: relative;
    //    `,
  };
});
