import { useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  setColorMode,
  selectAppColorMode,
  ColorMode,
} from '@webapp/redux/reducers/ui';

const useColorMode = () => {
  const dispatch = useAppDispatch();
  const colorMode = useAppSelector(selectAppColorMode);

  useEffect(() => {
    // sync color mode from redux with DOM body attr
    if (document.body.dataset.theme !== colorMode) {
      document.body.dataset.theme = colorMode;
    }
  }, [colorMode]);

  return {
    colorMode,
    changeColorMode: (newColorMode: ColorMode) =>
      dispatch(setColorMode(newColorMode)),
  };
};

export default useColorMode;
