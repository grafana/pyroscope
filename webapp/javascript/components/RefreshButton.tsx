import React from 'react';
import Button from '@webapp/ui/Button';

import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { useAppDispatch } from '@webapp/redux/hooks';
import { actions } from '@webapp/redux/reducers/continuous';

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
