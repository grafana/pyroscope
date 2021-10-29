import React from 'react';
import { Flamebearer } from '@models/flamebearer';
import clsx from 'clsx';
import { MenuItem } from '@szhsin/react-menu';
import useResizeObserver from '@react-hook/resize-observer';
import { Option } from 'prelude-ts';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';
import Highlight from './Highlight';
import Tooltip from './Tooltip';
import ContextMenu from './ContextMenu';
import { PX_PER_LEVEL } from './constants';
import Header from './Header';

interface FlamegraphProps {
  flamebearer: Flamebearer;
  focusedNode: ConstructorParameters<typeof Flamegraph>[2];
  fitMode: ConstructorParameters<typeof Flamegraph>[3];
  highlightQuery: ConstructorParameters<typeof Flamegraph>[4];
  zoom: ConstructorParameters<typeof Flamegraph>[5];

  onZoom: (bar: Option<{ i: number; j: number }>) => void;
  onFocusOnNode: (i: number, j: number) => void;

  onReset: () => void;
  isDirty: () => boolean;

  // the reason this is exposed as a parameter
  // is to not have to connect to the redux store from here
  ExportData: () => React.ReactElement;
}

export default function FlameGraphComponent(props: FlamegraphProps) {
  const canvasRef = React.useRef<HTMLCanvasElement>();
  const [flamegraph, setFlamegraph] = React.useState<Flamegraph>();

  const { flamebearer, focusedNode, fitMode, highlightQuery, zoom } = props;

  const { onZoom, onReset, isDirty, onFocusOnNode } = props;
  const { ExportData } = props;

  // rerender whenever the canvas size changes
  // eg window resize, or simply changing the view
  // to display the flamegraph isolated from the table
  useResizeObserver(canvasRef, () => {
    if (flamegraph) {
      renderCanvas();
    }
  });

  const onClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const opt = flamegraph.xyToBar(
      e.nativeEvent.offsetX,
      e.nativeEvent.offsetY
    );

    opt.match({
      // clicked on an invalid node
      None: () => {},
      Some: (bar) => {
        zoom.match({
          // there's no existing zoom
          // so just zoom on the clicked node
          None: () => {
            onZoom(opt);
          },

          // it's already zoomed
          Some: (z) => {
            // TODO there mya be stale props here...
            // we are clicking on the same node that's zoomed
            if (bar.i === z.i && bar.j === z.j) {
              // undo that zoom
              onZoom(Option.none());
            } else {
              onZoom(opt);
            }
          },
        });
      },
    });
  };

  const xyToHighlightData = (x: number, y: number) => {
    const opt = flamegraph.xyToBar(x, y);

    return opt.map((bar) => {
      return {
        left: canvasRef.current.offsetLeft + bar.x,
        top: canvasRef.current.offsetTop + bar.y,
        width: bar.width,
      };
    });
  };

  const xyToTooltipData = (x: number, y: number) => {
    return flamegraph.xyToBar(x, y);
  };

  // Context Menu stuff
  const xyToContextMenuItems = (x: number, y: number) => {
    const dirty = isDirty();
    const bar = flamegraph.xyToBar(x, y);

    const FocusItem = () => {
      const hoveredOnValidNode = bar.map(() => true).getOrElse(false);
      const onClick = bar
        .map((f) => onFocusOnNode.bind(null, f.i, f.j))
        .getOrElse(() => {});

      return (
        <MenuItem key="focus" disabled={!hoveredOnValidNode} onClick={onClick}>
          Focus on this subtree
        </MenuItem>
      );
    };

    return [
      <MenuItem key="reset" disabled={!dirty} onClick={onReset}>
        Reset View
      </MenuItem>,
      FocusItem(),
    ];
  };

  React.useEffect(() => {
    if (canvasRef.current) {
      const f = new Flamegraph(
        flamebearer,
        canvasRef.current,
        focusedNode,
        fitMode,
        highlightQuery,
        zoom
      );

      setFlamegraph(f);
    }
  }, [
    canvasRef.current,
    flamebearer,
    focusedNode,
    fitMode,
    highlightQuery,
    zoom,
  ]);

  const renderCanvas = () => {
    flamegraph.render();
  };

  React.useEffect(() => {
    if (flamegraph) {
      renderCanvas();
    }
  }, [flamegraph]);

  return (
    <>
      <div
        data-testid="flamegraph-view"
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
          />
        )}
        {flamegraph && (
          <Tooltip
            format={flamebearer.format}
            canvasRef={canvasRef}
            xyToData={xyToTooltipData as any /* TODO */}
            numTicks={flamebearer.numTicks}
            sampleRate={flamebearer.sampleRate}
            leftTicks={flamebearer.format === 'double' && flamebearer.leftTicks}
            rightTicks={
              flamebearer.format === 'double' && flamebearer.rightTicks
            }
            units={flamebearer.units}
          />
        )}

        {flamegraph && (
          <ContextMenu
            canvasRef={canvasRef}
            xyToMenuItems={xyToContextMenuItems}
          />
        )}
      </div>
    </>
  );
}
