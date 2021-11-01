import React from 'react';
import { Option } from 'prelude-ts';
import styles from './ContextMenuHighlight.module.css';

export interface HighlightProps {
  // probably the same as the bar height
  barHeight: number;

  node: Option<{ top: number; left: number; width: number }>;
}

const initialSyle: React.CSSProperties = {
  height: '0px',
  visibility: 'hidden',
};

/**
 * Highlight on the node that triggered the context menu
 */
export default function ContextMenuHighlight(props: HighlightProps) {
  const { node, barHeight } = props;
  const [style, setStyle] = React.useState(initialSyle);

  React.useEffect(
    () => {
      node.match({
        None: () => setStyle(initialSyle),
        Some: (data) =>
          setStyle({
            visibility: 'visible',
            height: `${barHeight}px`,
            ...data,
          }),
      });
    },
    // refresh callback functions when they change
    [node]
  );

  return (
    <div
      className={styles.highlightContextMenu}
      style={style}
      data-testid="flamegraph-highlight-contextmenu"
    />
  );
}
