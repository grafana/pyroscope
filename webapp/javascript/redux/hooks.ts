import { TypedUseSelectorHook, useDispatch, useSelector } from 'react-redux';
import type { RootState, AppDispatch } from './store';
import rootReducer from './reducers';

// Use throughout your app instead of plain `useDispatch` and `useSelector`
export const useAppDispatch = () => useDispatch<AppDispatch>();
export const useAppSelector: TypedUseSelectorHook<RootState> = useSelector;

// Until we migrate the old store to redux toolkit
// Let's use this to have some typing
export const useOldRootSelector: TypedUseSelectorHook<
  ReturnType<typeof rootReducer>
> = (fn: (a: ReturnType<typeof rootReducer>) => ShamefulAny) =>
  useAppSelector((state) => fn(state.root));
