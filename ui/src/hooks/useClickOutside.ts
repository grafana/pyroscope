import { useRef, useEffect } from 'react';

export function useClickOutside(
  ref: React.RefObject<HTMLElement | null>,
  cb: () => void,
) {
  const cbRef = useRef(cb);
  useEffect(() => {
    cbRef.current = cb;
  });
  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node))
        cbRef.current();
    };
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, [ref]);
}
