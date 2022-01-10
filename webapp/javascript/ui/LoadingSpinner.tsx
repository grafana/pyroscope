import React from 'react';
import Spinner from 'react-svg-spinner';

export default function LoadingSpinner() {
  // TODO ditch the library and create ourselves
  return (
    <span role="progressbar">
      <Spinner color="rgba(255,255,255,0.6)" size="20px" />
    </span>
  );
}
