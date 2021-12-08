import React from 'react';
import { PanelProps } from '@grafana/data';
import { stylesFactory, useTheme } from '@grafana/ui';
import { Option } from 'prelude-ts';
import { FitModes } from '../../webapp/javascript/util/fitMode';
import { SimpleOptions } from './types';
import Flamegraph from '../../webapp/javascript/components/FlameGraph/FlameGraphComponent/index';

type Props = PanelProps<SimpleOptions>;

function noopExportData() {
  return <div />;
}

// eslint-disable-next-line import/prefer-default-export
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
  const flamebearer = (
    data.series[data.series.length - 1].fields[0].values as any
  ).buffer[0];

  return (
    <>
      <div>
        <div className="flamegraph-wrapper">
          <Flamegraph
            flamebearer={flamebearer}
            zoom={Option.none()}
            focusedNode={Option.none()}
            highlightQuery=""
            onZoom={() => {}}
            onFocusOnNode={() => {}}
            onReset={() => {}}
            isDirty={() => false}
            fitMode={FitModes.HEAD}
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
