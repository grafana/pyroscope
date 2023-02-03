import { Flamebearer } from 'packages/pyroscope-models/src';
import React, { useMemo } from 'react';
import toGraphviz from '../../convert/toGraphviz';
import styles from './GraphVizPanel.module.scss';

// this is to make sure that graphviz-react is not used in node.js
let Graphviz = (obj: IGraphvizProps) => {
  if (obj) {
    return null;
  }
  return null;
};
interface IGraphvizProps {
  dot: string;
  options?: object;
  className?: string;
}

if (typeof process === 'undefined') {
  /* eslint-disable global-require */
  Graphviz = require('graphviz-react').Graphviz;
}

interface GraphVizPaneProps {
  flamebearer: Flamebearer;
}
export function GraphVizPane({ flamebearer }: GraphVizPaneProps) {
  // TODO(@petethepig): I don't understand what's going on with types here
  //   need to fix at some point
  const fb = flamebearer as ShamefulAny;

  // flamebearer
  const dot = fb.metadata?.format && fb.flamebearer?.levels;

  // Graphviz doesn't update position and scale value on rerender
  // so image sometimes moves out of the screen
  // to fix it we remounting graphViz component by updating key
  const key = `graphviz-pane-${fb?.appName || String(new Date().valueOf())}`;
  const dotValue = useMemo(() => {
    return toGraphviz(fb);
  }, [JSON.stringify(fb)]);

  return (
    <div className={styles.graphVizPane} key={key}>
      {dot ? (
        <Graphviz
          // options https://github.com/magjac/d3-graphviz#supported-options
          options={{
            zoom: true,
            width: '150%',
            height: '100%',
            scale: 1,
            // 'true' by default, but causes warning
            // https://github.com/magjac/d3-graphviz/blob/master/README.md#defining-the-hpcc-jswasm-script-tag
            useWorker: false,
          }}
          dot={dotValue}
        />
      ) : (
        <div>NO DATA</div>
      )}
    </div>
  );
}
