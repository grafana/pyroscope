import React from 'react';
import { Flamebearer } from '@models/flamebearer';
import clsx from 'clsx';
import styles from './canvas.module.css';
import Flamegraph from './Flamegraph';

//        <Graph
//          key="flamegraph-pane"
//          flamebearer={this.state.flamebearer}
//          format={this.parseFormat(this.state.flamebearer.format)}
//          view={this.state.view}
//          ExportData={ExportData}
//          query={this.state.highlightQuery}
//          fitMode={this.state.fitMode}
//          viewType={this.props.viewType}
//          topLevel={this.state.flamegraphConfigs.topLevel}
//          zoom={this.state.flamegraphConfigs.zoom}
//          selectedLevel={this.state.flamegraphConfigs.selectedLevel}
//          label={this.props.query}
//          onZoom={this.onFlamegraphZoom}
//          onReset={this.onReset}
//          isDirty={this.isDirty}

interface FlamegraphProps {
  flamebearer: Flamebearer;
  fitMode: typeof Flamegraph.fitMode;
  zoom: typeof Flamegraph.zoom;
  topLevel: typeof Flamegraph.topLevel;
  selectedLevel: typeof Flamegraph.selectedLevel;
  query: typeof Flamegraph.highlightQuery;

  // TODO
  // format: any;
  viewType: string; // TODO

  onZoom: (i: number, j: number) => void;
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

  const onClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const { i, j } = flamegraph.xyToBar(
      e.nativeEvent.offsetX,
      e.nativeEvent.offsetY
    );

    props.onZoom(i, j);
  };

  React.useEffect(() => {
    if (!canvasRef.current) {
      return () => {};
    }

    const f = new Flamegraph(
      props.flamebearer,
      canvasRef.current,
      props.topLevel,
      props.selectedLevel,
      props.fitMode,
      props.query,
      props.zoom
    );
    //
    setFlamegraph(f);
    //
    // do we need to clear the flamegraph?
    return () => {};
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
      console.log('rendering');
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
      </div>
    </>
  );
}
