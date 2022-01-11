// TODO reenable spreading lint
/* eslint-disable react/jsx-props-no-spreading */
import React, { useState, useEffect } from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import { appNameToQuery, queryToAppName } from '@utils/query';
import {
  selectAppNames,
  selectAppNamesState,
  reloadAppNames,
} from '@pyroscope/redux/reducers/newRoot';
import LoadingSpinner from '@ui/LoadingSpinner';
import Button from '@ui/Button';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import Dropdown, { MenuItem, FocusableItem } from '@ui/Dropdown';
import { setQuery } from '../redux/actions';
import styles from './NameSelector.module.scss';

const defKey = 'Select an app...';

function NameSelector(props) {
  const { actions, names, query } = props;
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNames = useAppSelector(selectAppNames);

  const [filter, setFilter] = useState('');

  const selectAppName = (name: string) => {
    const query = appNameToQuery(name);
    actions.setQuery(query);
  };

  const dispatch = useAppDispatch();

  // if there's no query set
  // set the first app as the default (if exists)
  useEffect(() => {
    if (!query) {
      const first = appNames[0];
      if (first) {
        const query = appNameToQuery(first);
        actions.setQuery(query);
      }
    }
  }, [query]);

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
  )) as any;

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

const mapStateToProps = (state) => ({
  ...state.root,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      setQuery,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(NameSelector);
