import React from 'react';
import { useDispatch } from 'react-redux';
import Button from '@ui/Button';

import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { refresh } from '../redux/actions';

function RefreshButton() {
  const dispatch = useDispatch();
  return (
    <Button
      data-testid="refresh-btn"
      icon={faSyncAlt}
      onClick={() => dispatch(refresh())}
    />
  );
}

export default RefreshButton;
