import React, { useState, useEffect, useMemo } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { faSlidersH } from '@fortawesome/free-solid-svg-icons/faSlidersH';
import cl from 'classnames';

import type { App } from '@webapp/models/app';
import { queryFromAppName, queryToAppName, Query } from '@webapp/models/query';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  actions,
  reloadAppNames,
  selectQueries,
  selectAppNamesState,
} from '@webapp/redux/reducers/continuous';
import Button from '@webapp/ui/Button';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';
import Input from '@webapp/ui/Input';
import SelectButton from './SelectButton';
import { SPY_NAMES_ICONS } from './SpyNameIcons';
import styles from './AppSelector.module.scss';

interface AppSelectorProps {
  // Comparison/Diff View pages provide {onSelectedName} func which
  // handle propagating query to left/right flamegraphs
  onSelectedName?: (name: Query) => void;

  filterApp?: (names: string) => boolean;
}

const AppSelector = ({
  onSelectedName,
  filterApp = () => true,
}: AppSelectorProps) => {
  const dispatch = useAppDispatch();
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNamesData = appNamesState.data.filter((v) => filterApp(v.name));

  const { query } = useAppSelector(selectQueries);
  const app: App = queryToAppName(query).mapOr(
    { name: '', spyName: 'unknown', units: 'unknown' },
    (q) =>
      appNamesData.find((v) => v.name === q) || {
        name: '',
        spyName: 'unknown',
        units: 'unknown',
      }
  );

  const selectAppName = (name: string) => {
    const appNameQuery = queryFromAppName(name);
    if (onSelectedName) {
      onSelectedName(appNameQuery);
    } else {
      dispatch(actions.setQuery(appNameQuery));
    }
  };

  return (
    <div className={styles.container}>
      Application:&nbsp;
      <SelectorModalWithToggler
        selectAppName={selectAppName}
        appNamesData={appNamesData}
        app={app}
      />
      <Button
        aria-label="Refresh Apps"
        icon={faSyncAlt}
        onClick={() => dispatch(reloadAppNames())}
        className={styles.refreshButton}
      />
      {appNamesState.type === 'reloading' && <LoadingSpinner />}
    </div>
  );
};

export default AppSelector;

const DELIMITER = '.';
const isGroupMember = (groupName: string, name: string) =>
  name.indexOf(groupName) === 0 &&
  (name[groupName.length] === DELIMITER || name.length === groupName.length);

const getGroupMembers = (names: string[], name: string) =>
  names.filter((a) => isGroupMember(name, a));

const getGroupNameFromAppName = (groups: string[], fullName: string) =>
  groups.filter((g) => isGroupMember(g, fullName))[0] || '';

const getGroups = (filteredAppNames: string[]) => {
  const allGroups = filteredAppNames.map((i) => {
    const arr = i.split(DELIMITER);
    const cutProfileType = arr.length > 1 ? arr.slice(0, -1) : arr;
    return cutProfileType.join(DELIMITER);
  });

  const uniqGroups = Array.from(new Set(allGroups));

  const dedupedUniqGroups = uniqGroups.filter((x) => {
    return !uniqGroups.find((y) => x !== y && isGroupMember(y, x));
  });

  const groupOrApp = dedupedUniqGroups.map((u) => {
    const appNamesEntries = getGroupMembers(filteredAppNames, u);

    return appNamesEntries.length > 1 ? u : appNamesEntries?.[0];
  });

  return groupOrApp;
};

interface SelectorModalWithTogglerProps {
  app: App;
  appNamesData: App[];
  selectAppName: (name: string) => void;
}

const SelectorModalWithToggler = ({
  app,
  appNamesData,
  selectAppName,
}: SelectorModalWithTogglerProps) => {
  const [filters, setFilters] = useState({
    search: '',
    spyName: '',
    profileType: '',
  });
  const [isModalOpen, setModalOpenStatus] = useState(false);

  // selected is an array of strings
  //  0 corresponds to string of group / app name selected in the left pane
  //  1 corresponds to string of app name selected in the right pane
  const [selected, setSelected] = useState<string[]>([]);
  const filteredApps = useMemo(
    () =>
      appNamesData.filter((n) => {
        const { search, spyName, profileType } = filters;
        let matchFilters = true;

        if (search && matchFilters) {
          matchFilters = n.name
            .toLowerCase()
            .includes(filters.search.trim().toLowerCase());
        }

        if (spyName && matchFilters) {
          matchFilters = n.spyName === spyName;
        }

        if (profileType && matchFilters) {
          matchFilters = n.name.includes(profileType);
        }

        return matchFilters;
      }),
    [filters, appNamesData]
  );

  const filteredAppNames = filteredApps.map((v) => v.name);
  const { spyNames, profileTypes } = appNamesData.reduce(
    (acc, v) => {
      if (acc.spyNames.indexOf(v.spyName) === -1) {
        acc.spyNames.push(v.spyName);
      }

      const propfileType = v.name.split('.').pop() as string;
      if (acc.profileTypes.indexOf(propfileType) === -1) {
        acc.profileTypes.push(propfileType);
      }

      return acc;
    },
    { spyNames: [] as string[], profileTypes: [] as string[] }
  );

  const groups = useMemo(() => getGroups(filteredAppNames), [filteredAppNames]);
  const profiles = useMemo(() => {
    if (!selected?.[0]) {
      return [];
    }

    const filtered = getGroupMembers(filteredAppNames, selected?.[0]);

    if (filtered.length > 1) {
      return filtered;
    }

    return [];
  }, [selected, groups, filteredAppNames]);

  const onSelect = ({ index, name }: { index: number; name: string }) => {
    const filtered = getGroupMembers(filteredAppNames, name);

    if (filtered.length === 1 || index === 1) {
      selectAppName(filtered?.[0]);
      setModalOpenStatus(false);
    }

    const arr = Array.from(selected);

    if (index === 0 && arr?.length > 1) {
      arr.pop();
    }

    arr[index] = name;

    setSelected(arr);
  };

  useEffect(() => {
    if (groups.indexOf(app.name) !== -1) {
      setSelected([app.name]);
      setModalOpenStatus(false);
    } else {
      setSelected([getGroupNameFromAppName(groups, app.name), app.name]);
    }
  }, [app.name]);

  const listHeight = useMemo(() => {
    const height = (window?.innerHeight || 0) - 160;

    const listRequiredHeight =
      // 35 is list item height
      Math.max(groups?.length || 0, profiles?.length || 0) * 35;

    if (height && listRequiredHeight) {
      return height >= listRequiredHeight ? 'auto' : `${height}px`;
    }

    return 'auto';
  }, [groups, profiles]);

  const handleFilterChange = (
    k: 'search' | 'spyName' | 'profileType',
    v: string
  ) => {
    setFilters((prevFilters) => {
      if (prevFilters[k] === v) {
        return { ...prevFilters, [k]: '' };
      }
      return { ...prevFilters, [k]: v };
    });
  };

  return (
    <ModalWithToggle
      isModalOpen={isModalOpen}
      setModalOpenStatus={setModalOpenStatus}
      modalClassName={styles.appSelectorModal}
      modalHeight={listHeight}
      noDataEl={
        !filteredAppNames?.length ? (
          <div data-testid="app-selector-no-data" className={styles.noData}>
            No Data
          </div>
        ) : null
      }
      toggleText={app.name || 'Select application'}
      headerEl={
        <div className={styles.header}>
          <div>
            <div className={styles.headerTitle}>SELECT APPLICATION</div>
            <Input
              name="application seach"
              type="text"
              placeholder="Type an app"
              value={filters.search}
              onChange={(e) => handleFilterChange('search', e.target.value)}
              className={styles.searchInput}
              testId="application-search"
            />
          </div>
          <div>
            <div className={styles.headerTitle}>
              <FontAwesomeIcon icon={faSlidersH} /> FILTERS
            </div>
            <div>
              <div className={styles.filter}>
                <div className={styles.filterName}>Language</div>
                <div className={styles.iconsContainer}>
                  {spyNames.map((v) => (
                    <button
                      type="button"
                      key={v}
                      data-testid={v}
                      className={cl(styles.icon, {
                        [styles.active]: v === filters.spyName,
                      })}
                      onClick={() => handleFilterChange('spyName', v)}
                    >
                      {SPY_NAMES_ICONS[v]}
                    </button>
                  ))}
                </div>
              </div>
              <div className={styles.filter}>
                <div className={styles.filterName}>Profile type</div>
                <div className={styles.profileTypesContainer}>
                  {profileTypes.map((v) => (
                    <button
                      type="button"
                      key={v}
                      className={cl(styles.profileType, {
                        [styles.active]: v === filters.profileType,
                      })}
                      onClick={() => handleFilterChange('profileType', v)}
                    >
                      {v}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      }
      leftSideEl={groups.map((name) => (
        <SelectButton
          name={name}
          onClick={() => onSelect({ index: 0, name })}
          fullList={filteredAppNames}
          isSelected={selected?.[0] === name}
          key={name}
        />
      ))}
      rightSideEl={profiles.map((name) => (
        <SelectButton
          name={name}
          onClick={() => onSelect({ index: 1, name })}
          fullList={filteredAppNames}
          isSelected={selected?.[1] === name}
          key={name}
        />
      ))}
    />
  );
};
