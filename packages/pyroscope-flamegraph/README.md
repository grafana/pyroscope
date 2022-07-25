# Pyroscope Flamegraph
This is a component which allows for you to render flamegraphs in your website or application.
While this is typically used for profiling data this can also be used to render tracing data as well

## Rendering Profiling Data

Import the CSS
```
import '@pyroscope/flamegraph/dist/index.css';
```

Import the `FlamegraphRenderer` component

```
import { FlamegraphRenderer, Box } from '@pyroscope/flamegraph';

const SimpleTree = {
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

export const Flamegraph = () => {
  return (
    <FlamegraphRenderer
      profile={SimpleTree}
      onlyDisplay="flamegraph"
      showToolbar={false}
    />
  );
};
```

We recommend wrapping your component around a `Box` to give it some padding.
```
<Box>
  <FlamegraphRenderer
    profile={SimpleTree}
    onlyDisplay="flamegraph"
    showToolbar={false}
  />
</Box>
```

## Rendering Tracing Data
Note: Currently Pyroscope only supports rendering tracing data from Jaeger.

```
import { FlamegraphRenderer, convertJaegerTraceToProfile } from "@pyroscope/flamegraph";
import "@pyroscope/flamegraph/dist/index.css";

let trace = jaegerTrace.data[0];
let convertedProfile = convertJaegerTraceToProfile(trace);

function App() {
  return (
    <div>
      <h1>Trace Flamegraph</h1>
      <FlamegraphRenderer
        profile={convertedProfile}
        onlyDisplay="flamegraph"
        showToolbar={true}
      />
    </div>
  );
}
```
