import { useCallback, useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import { setColorMode, selectAppColorMode } from '@pyroscope/redux/reducers/ui';

/** Obtain the current color mode (light|dark) and ensure all representations of it are in sync
 *
 * Some elements of style rely on the `<body>` tag's `data-theme` attribute
 * (accessibile via document.body.dataset.theme)
 * while others must consult with the color mode set in the redux store.
 *
 * This hook ensures that any change in the redux store is reflected in the body theme,
 * and vice versa.
 */
const useColorMode = () => {
  const dispatch = useAppDispatch();
  const colorMode = useAppSelector(selectAppColorMode);
  const { body } = document;

  /** This callback updates the color mode in the redux store. */
  const changeColorMode = useCallback(
    (newColorMode: 'light' | 'dark') => dispatch(setColorMode(newColorMode)),
    [dispatch]
  );

  // This effect sets up an observer for changes in the `<body>` `data-theme` attr
  // which updates the redux value to keep them in sync.
  useEffect(() => {
    const observer = new MutationObserver((mutationsList) => {
      const mutation = mutationsList[0];

      if (mutation.type === 'attributes' && body.dataset.theme !== colorMode) {
        changeColorMode(
          body.dataset.theme === 'light' || body.dataset.theme === 'dark'
            ? body.dataset.theme
            : 'dark'
        );
      }
    });

    if (body) {
      observer.observe(body, {
        attributes: true,
        attributeFilter: ['data-theme'],
        childList: false,
        subtree: false,
      });
    }

    return () => {
      observer.disconnect();
    };
  }, [body, changeColorMode, colorMode]);

  // This effect will update the `<body>` tag's `data-theme` attribute
  // to ensure that it matches the color mode from the redux store.
  useEffect(() => {
    // sync redux value with DOM body attr
    if (body.dataset.theme !== colorMode) {
      body.dataset.theme = colorMode;
    }
  }, [colorMode, body]);

  return {
    colorMode,
    changeColorMode,
  };
};

export default useColorMode;
