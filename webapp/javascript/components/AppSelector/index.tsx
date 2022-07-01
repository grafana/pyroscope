import React, { useState } from 'react';
import { queryFromAppName, queryToAppName, Query } from '@webapp/models/query';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  selectIsLoadingData,
  actions,
  selectAppNames,
  reloadAppNames,
  selectQueries,
} from '@webapp/redux/reducers/continuous';
import { useLocation } from 'react-router-dom';
import { PAGES } from '@webapp/pages/constants';
import classNames from 'classnames';
import Spinner from 'react-svg-spinner';
import Button from '@webapp/ui/Button';
import OutsideClickHandler from 'react-outside-click-handler';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import SelectorModal from './SelectorModal';
import styles from './AppSelector.module.scss';

interface AppSelectorProps {
  onSelectedName?: (name: Query) => void;
}

const TOGGLE_BTN_ID = 'toggle_button';

const AppSelector = ({ onSelectedName }: AppSelectorProps) => {
  const dispatch = useAppDispatch();
  const location = useLocation();
  const appNames = useAppSelector(selectAppNames);
  const { query } = useAppSelector(selectQueries);
  const appName = queryToAppName(query).mapOr('', (q) =>
    appNames.indexOf(q) !== -1 ? q : ''
  );
  const isLoadingData = useAppSelector((state) =>
    selectIsLoadingData(state, location?.pathname as PAGES)
  );

  const [selectorOpened, toggleSelector] = useState(false);

  const selectAppName = (name: string) => {
    const appNameQuery = queryFromAppName(name);

    if (onSelectedName) {
      onSelectedName(appNameQuery);
    } else {
      dispatch(actions.setQuery(appNameQuery));
    }

    toggleSelector(false);
  };

  const handleClickOutsile = (e: MouseEvent) => {
    if ((e.target as { id?: string })?.id !== TOGGLE_BTN_ID) {
      toggleSelector(false);
    }
  };

  return (
    <div className={styles.container}>
      Application:&nbsp;
      <button
        id={TOGGLE_BTN_ID}
        className={styles.toggleButton}
        onClick={() => toggleSelector(!selectorOpened)}
        type="button"
      >
        {appName || 'Select application'}
      </button>
      <Button
        aria-label="Refresh Apps"
        icon={faSyncAlt}
        onClick={() => dispatch(reloadAppNames())}
      />
      <div
        className={classNames('spinner-container', {
          visible: isLoadingData,
          loaded: !isLoadingData,
        })}
      >
        {isLoadingData && <Spinner color="rgba(255,255,255,0.6)" size="20px" />}
      </div>
      <OutsideClickHandler onOutsideClick={handleClickOutsile}>
        <SelectorModal
          selectAppName={selectAppName}
          appNames={appNames}
          visible={selectorOpened}
          appName={appName}
        />
      </OutsideClickHandler>
    </div>
  );
};

export default AppSelector;
