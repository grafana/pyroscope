// TODO reenable spreading lint
/* eslint-disable react/jsx-props-no-spreading */
import React, { useState, useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import { appNameToQuery, queryToAppName } from '@utils/query';
import {
  actions,
  selectContinuousState,
  selectAppNames,
  selectAppNamesState,
  reloadAppNames,
} from '@pyroscope/redux/reducers/continuous';
import LoadingSpinner from '@ui/LoadingSpinner';
import Button from '@ui/Button';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import Dropdown, { MenuItem, FocusableItem } from '@ui/Dropdown';
import styles from './NameSelector.module.scss';

function NameSelector() {
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNames = useAppSelector(selectAppNames);
  const dispatch = useAppDispatch();
  const { query } = useAppSelector(selectContinuousState);

  const [filter, setFilter] = useState('');

  const selectAppName = (name: string) => {
    const query = appNameToQuery(name);

    dispatch(actions.setQuery(query));
  };

  const selectedValue = queryToAppName(query).mapOr('', (q) => {
    if (appNames.indexOf(q) !== -1) {
      return q;
    }
    return '';
  });

  const filterOptions = (n: string) => {
    const f = filter.trim().toLowerCase();
    return n.toLowerCase().includes(f);
  };

  // TODO figure out this any
  const options = appNames.filter(filterOptions).map((name) => (
    <MenuItem
      key={name}
      value={name}
      onClick={() => selectAppName(name)}
      className={selectedValue === name ? 'active' : ''}
    >
      {name}
    </MenuItem>
  )) as ShamefulAny;

  const noApp =
    appNames.length > 0 ? null : <MenuItem>No App available</MenuItem>;

  return (
    <div className={styles.container}>
      Application:&nbsp;
      <Dropdown
        label="Select application"
        data-testid="app-name-selector"
        value={selectedValue}
        overflow="auto"
        position="anchor"
        menuButtonClassName={styles.menuButton}
      >
        {noApp}
        <FocusableItem>
          {({ ref }) => (
            <input
              ref={ref}
              type="text"
              placeholder="Type an app"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
          )}
        </FocusableItem>

        {options}
      </Dropdown>
      <Button
        aria-label="Refresh Apps"
        icon={faSyncAlt}
        onClick={() => {
          dispatch(reloadAppNames());
        }}
      />
      <Loading {...appNamesState} />
    </div>
  );
}

function Loading({ type }: ReturnType<typeof selectAppNamesState>) {
  switch (type) {
    case 'reloading': {
      return <LoadingSpinner />;
    }

    default: {
      return null;
    }
  }
}
export default NameSelector;
