import createSlicer from "redux-localstorage/lib/createSlicer";
import mergeState from "redux-localstorage/lib/util/mergeState";

export default function updateUrl(paths, config) {
  const cfg = {
    merge: mergeState,
    slicer: createSlicer,
    serialize: JSON.stringify,
    deserialize: JSON.parse,
    ...config,
  };

  const { merge, slicer, serialize, deserialize } = cfg;

  return (next) => (reducer, initialState, enhancer) => {
    if (typeof initialState === "function" && typeof enhancer === "undefined") {
      enhancer = initialState;
      initialState = undefined;
    }

    let persistedState;
    let finalInitialState;

    try {
      const queryString = window.location.search;
      const urlParams = new URLSearchParams(queryString);
      const persistedState = {};

      paths.forEach((x) => {
        const val = urlParams.get(x);
        if (val) {
          persistedState[x] = val.startsWith("json:")
            ? JSON.parse(val.replace("json:", ""))
            : val;
        }
      });

      finalInitialState = merge(initialState, persistedState);
    } catch (e) {
      console.warn("Failed to retrieve initialize state from URL:", e);
    }

    const store = next(reducer, finalInitialState, enhancer);
    const slicerFn = slicer(paths);

    store.subscribe(() => {
      const state = store.getState();
      const subset = slicerFn(state);

      try {
        const queryString = window.location.search;
        const urlParams = new URLSearchParams(queryString);
        paths.forEach((x) => {
          if (state[x]) {
            const val =
              typeof state[x] === "string"
                ? state[x]
                : `json:${JSON.stringify(state[x])}`;
            urlParams.set(x, val);
          }
        });
        history.pushState({}, "title", `/?${urlParams.toString()}`);
      } catch (e) {
        console.warn("Unable to persist state to URL:", e);
      }
    });

    return store;
  };
}
