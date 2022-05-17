import React from 'react';
import Button from '@webapp/ui/Button';

import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { useAppDispatch } from '@webapp/redux/hooks';
import { actions } from '@webapp/redux/reducers/continuous';
import debounce from 'lodash.debounce';

function RefreshButton() {
  const dispatch = useAppDispatch();
  const refresh = debounce(() => {
    dispatch(actions.refresh());
  }, 500);
  return (
    <Button data-testid="refresh-btn" icon={faSyncAlt} onClick={refresh} />
  );
}

export default RefreshButton;
