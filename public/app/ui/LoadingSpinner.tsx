import React from 'react';
import Spinner from 'react-svg-spinner';

interface LoadingSpinnerProps {
  className?: string;
  size?: string;
}

export default function LoadingSpinner({
  className,
  size = '20px',
}: LoadingSpinnerProps) {
  // TODO ditch the library and create ourselves
  return (
    <span role="progressbar" className={className}>
      <Spinner color="rgba(255,255,255,0.6)" size={size} />
    </span>
  );
}
