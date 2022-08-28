import React from 'react';
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
import Spinner from 'react-svg-spinner';

interface LoadingSpinnerProps {
  className?: string;
}

export default function LoadingSpinner({ className }: LoadingSpinnerProps) {
  // TODO ditch the library and create ourselves
  return (
    <span role="progressbar" className={className}>
      <Spinner color="rgba(255,255,255,0.6)" size="20px" />
    </span>
  );
}
