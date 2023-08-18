import React from 'react';
import Button from '@pyroscope/ui/Button';

import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import { actions } from '@pyroscope/redux/reducers/continuous';

function RefreshButton() {
  const dispatch = useAppDispatch();
  return (
    <Button
      data-testid="refresh-btn"
      icon={faSyncAlt}
      onClick={() => dispatch(actions.refresh())}
    />
  );
}

export default RefreshButton;
