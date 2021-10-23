import React from 'react';
import { Flamebearer } from '@models/flamebearer';
import clsx from 'clsx';
import { MenuItem } from '@szhsin/react-menu';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';
import Highlight from './Highlight';
import Tooltip from './Tooltip';
import ContextMenu from './ContextMenu';
import { PX_PER_LEVEL } from './constants';

interface FlamegraphProps {
  flamebearer: Flamebearer;
  fitMode: typeof Flamegraph.fitMode;
  zoom: typeof Flamegraph.zoom; // TODO call it zoom level?
  topLevel: typeof Flamegraph.topLevel;
  selectedLevel: typeof Flamegraph.selectedLevel;
  query: typeof Flamegraph.highlightQuery;

  // TODO
  // format: any;
  viewType: string; // TODO

  onZoom: (i: number, j: number) => void;

  onReset: () => void;
  isDirty: () => boolean;
}

export default function FlameGraphComponent(props: FlamegraphProps) {
  const canvasRef = React.useRef();
  const [flamegraph, setFlamegraph] = React.useState<Flamegraph>();

  const {
    flamebearer,
    topLevel,
    selectedLevel,
    fitMode,
    query,
    zoom,
    viewType,
  } = props;

  const { onZoom } = props;
  const { onReset, isDirty } = props;

  const onClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const { i, j } = flamegraph.xyToBar(
      e.nativeEvent.offsetX,
      e.nativeEvent.offsetY
    );

    onZoom(i, j);
  };

  const xyToHighlightData = (x: number, y: number) => {
    const bar = flamegraph.xyToBarPosition(x, y);

    return {
      left: canvasRef?.current?.offsetLeft + bar.x,
      top: canvasRef?.current?.offsetTop + bar.y,
      width: bar.width,
    };
  };

  const xyToTooltipData = (format: string, x: number, y: number) => {
    return flamegraph.xyToBarData(x, y);
  };

  // Context Menu stuff
  const xyToContextMenuItems = (x: number, y: number) => {
    const dirty = isDirty();

    //
    //      <MenuItem key="focus" onClick={() => this.focusOnNode(x, y)}>
    //        Focus
    //      </MenuItem>,
    return [
      <MenuItem key="reset" disabled={!dirty} onClick={onReset}>
        Reset View
      </MenuItem>,
    ];
  };

  // this level of indirection is required
  // otherwise may get stale props
  // eg. thinking that a zoomed flamegraph is not zoomed
  const isWithinBounds = (x: number, y: number) =>
    flamegraph.isWithinBounds(x, y);

  React.useEffect(() => {
    if (canvasRef.current) {
      const f = new Flamegraph(
        flamebearer,
        canvasRef.current,
        topLevel,
        selectedLevel,
        fitMode,
        query,
        zoom
      );

      setFlamegraph(f);
    }
  }, [
    canvasRef.current,
    flamebearer,
    topLevel,
    selectedLevel,
    fitMode,
    query,
    zoom,
  ]);

  React.useEffect(() => {
    if (flamegraph) {
      flamegraph.render();
    }
  }, [flamegraph]);
  return (
    <>
      <div
        className={clsx('flamegraph-pane', {
          'vertical-orientation': viewType === 'double',
        })}
      >
        <div />
        <div>
          <canvas
            height="0"
            data-testid="flamegraph-canvas"
            className={`flamegraph-canvas ${styles.hover}`}
            ref={canvasRef}
            onClick={onClick}
          />
        </div>
        {flamegraph && (
          <Highlight
            barHeight={PX_PER_LEVEL}
            canvasRef={canvasRef}
            xyToHighlightData={xyToHighlightData}
            isWithinBounds={isWithinBounds}
          />
        )}
        {flamegraph && (
          <Tooltip
            format={flamebearer.format}
            canvasRef={canvasRef}
            xyToData={xyToTooltipData}
            isWithinBounds={isWithinBounds}
            numTicks={flamebearer.numTicks}
            sampleRate={flamebearer.sampleRate}
            leftTicks={flamebearer.leftTicks}
            rightTicks={flamebearer.rightTicks}
            units={flamebearer.units}
          />
        )}

        <ContextMenu
          canvasRef={canvasRef}
          xyToMenuItems={xyToContextMenuItems}
        />
      </div>
    </>
  );
}
