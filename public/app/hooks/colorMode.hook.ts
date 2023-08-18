import { useCallback, useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import { setColorMode, selectAppColorMode } from '@pyroscope/redux/reducers/ui';

const useColorMode = () => {
  const dispatch = useAppDispatch();
  const colorMode = useAppSelector(selectAppColorMode);
  const { body } = document;

  const changeColorMode = useCallback(
    (newColorMode: 'light' | 'dark') => dispatch(setColorMode(newColorMode)),
    [dispatch]
  );

  useEffect(() => {
    // sync data-theme attr with redux value
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

  useEffect(() => {
    // sync redux value with DOM body attr
    if (body.dataset.theme !== colorMode) {
      body.dataset.theme = colorMode;
    }
  }, [colorMode, body, body.dataset]);

  return {
    colorMode,
    changeColorMode,
  };
};

export default useColorMode;
