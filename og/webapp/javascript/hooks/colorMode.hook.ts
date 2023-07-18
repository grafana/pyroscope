import { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import { setColorMode, selectAppColorMode } from '@webapp/redux/reducers/ui';

const useColorMode = () => {
  const dispatch = useAppDispatch();
  const colorMode = useAppSelector(selectAppColorMode);
  const { body } = document;

  const changeColorMode = (newColorMode: 'light' | 'dark') =>
    dispatch(setColorMode(newColorMode));

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
  }, []);

  useEffect(() => {
    // sync redux value with DOM body attr
    if (body.dataset.theme !== colorMode) {
      body.dataset.theme = colorMode;
    }
  }, [colorMode]);

  return {
    colorMode,
    changeColorMode,
  };
};

export default useColorMode;
