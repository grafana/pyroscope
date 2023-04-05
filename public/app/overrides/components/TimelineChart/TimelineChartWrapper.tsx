import { useEffect, useRef } from 'react';

export default function () {
  const ref = useRef<HTMLDivElement>(null);

  // Since this element is inside a <Box>, also make the box hidden
  useEffect(() => {
    const parentElement = ref.current?.parentElement?.parentElement;
    if (parentElement) {
      parentElement.style.display = 'none';
    }
  }, [ref.current]);

  return <div ref={ref}></div>;
}
