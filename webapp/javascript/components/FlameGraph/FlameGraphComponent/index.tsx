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
import Header from './Header';

interface FlamegraphProps {
  flamebearer: Flamebearer;
  topLevel: ConstructorParameters<typeof Flamegraph>[2];
  selectedLevel: ConstructorParameters<typeof Flamegraph>[3];
  fitMode: ConstructorParameters<typeof Flamegraph>[4];
  query: ConstructorParameters<typeof Flamegraph>[5];
  zoom: ConstructorParameters<typeof Flamegraph>[6]; // TODO call it zoom level?

  onZoom: (i: number, j: number) => void;

  onReset: () => void;
  isDirty: () => boolean;

  // the reason this is exposed as a parameter
  // is to not have to connect to the redux store from here
  ExportData: () => React.ReactElement;
}

export default function FlameGraphComponent(props: FlamegraphProps) {
  const canvasRef = React.useRef<HTMLCanvasElement>();
  const [flamegraph, setFlamegraph] = React.useState<Flamegraph>();

  const { flamebearer, topLevel, selectedLevel, fitMode, query, zoom } = props;

  const { onZoom, onReset, isDirty } = props;
  const { ExportData } = props;

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
      left: canvasRef.current.offsetLeft + bar.x,
      top: canvasRef.current.offsetTop + bar.y,
      width: bar.width,
    };
  };

  const xyToTooltipData = (x: number, y: number) => {
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

  const renderCanvas = () => {
    flamegraph.render();
  };

  React.useEffect(() => {
    if (!flamegraph) {
      return () => {};
    }

    window.addEventListener('resize', () => {
      renderCanvas();
    });

    renderCanvas();

    return () => {
      window.removeEventListener('resize', flamegraph.render);
    };
  }, [flamegraph]);

  return (
    <>
      <div
        className={clsx('flamegraph-pane', {
          'vertical-orientation': flamebearer.format === 'double',
        })}
      >
        <Header
          format={flamebearer.format}
          units={flamebearer.units}
          ExportData={ExportData}
        />

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
            xyToData={xyToTooltipData as any /* TODO */}
            isWithinBounds={isWithinBounds}
            numTicks={flamebearer.numTicks}
            sampleRate={flamebearer.sampleRate}
            leftTicks={flamebearer.format === 'double' && flamebearer.leftTicks}
            rightTicks={
              flamebearer.format === 'double' && flamebearer.rightTicks
            }
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
