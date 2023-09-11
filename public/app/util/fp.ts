import { Maybe, Result } from 'true-myth';
import type { Unwrapped } from 'true-myth/maybe';

// Should be used in situation where we are absolutely
// want to throw an exception
// eg in tests
// DO NOT USE NORMALLY IN CODE
const throwUnwrapErr = () => {
  throw new Error('Failed to unwrap');
};

export { Maybe, Result, throwUnwrapErr };
export type { Unwrapped as UnwrapMaybe };
