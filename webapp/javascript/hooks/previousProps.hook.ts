import { useEffect, useRef } from 'react';

const usePreviousProps: ShamefulAny = (value: ShamefulAny) => {
  const ref = useRef();
  useEffect(() => {
    ref.current = value;
  });
  return ref.current;
};

export default usePreviousProps;
