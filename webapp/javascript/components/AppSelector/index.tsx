import React, { useState, useMemo } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import { faSlidersH } from '@fortawesome/free-solid-svg-icons/faSlidersH';
import { faUndo } from '@fortawesome/free-solid-svg-icons/faUndo';
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
import { Tooltip } from '@webapp/ui/Tooltip';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';
import Input from '@webapp/ui/Input';
import { SpyNameFirstClassType } from '@pyroscope/models/src';
import SelectButton from './SelectButton';
import { SPY_NAMES_TOOLTIPS, SPY_NAMES_ICONS } from './SpyNameIcons';
import useFilters from './useFilters';
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
  const { query } = useAppSelector(selectQueries);

  const apps = appNamesState.data.filter((v) => filterApp(v.name));
  const app: App = queryToAppName(query).mapOr(
    { name: '', spyName: 'unknown', units: 'unknown' },
    (q) =>
      apps.find((v) => v.name === q) || {
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
        apps={apps}
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

const getSelectedApp = (
  appName: string,
  groups: string[],
  selected: string[]
) => {
  const isFirstLevel = !!(groups.indexOf(appName) !== -1);

  if (selected.length !== 0) {
    return selected;
  }

  if (isFirstLevel) {
    return [appName];
  }
  return [getGroupNameFromAppName(groups, appName), appName];
};

interface SelectorModalWithTogglerProps {
  app: App;
  apps: App[];
  selectAppName: (name: string) => void;
}

const SelectorModalWithToggler = ({
  app,
  apps,
  selectAppName,
}: SelectorModalWithTogglerProps) => {
  const [isModalOpen, setModalOpenStatus] = useState(false);
  const {
    filters,
    filteredAppNames,
    spyNameValues,
    profileTypeValues,
    handleFilterChange,
    resetClickableFilters,
  } = useFilters(apps);

  // selected is an array of strings
  //  0 corresponds to string of group / app name selected in the left pane
  //  1 corresponds to string of app name selected in the right pane
  const [selected, setSelected] = useState<string[]>([]);

  const groups = useMemo(() => getGroups(filteredAppNames), [filteredAppNames]);
  const selectedApp = getSelectedApp(app.name, groups, selected);

  const profilesNames = useMemo(() => {
    if (!selectedApp?.[0]) {
      return [];
    }

    const filtered = getGroupMembers(filteredAppNames, selectedApp?.[0]);

    if (filtered.length > 1) {
      return filtered;
    }

    return [];
  }, [selectedApp, groups, filteredAppNames]);

  const onSelect = ({ index, name }: { index: number; name: string }) => {
    const filtered = getGroupMembers(filteredAppNames, name);

    if (filtered.length === 1 || index === 1) {
      selectAppName(filtered?.[0]);
      setModalOpenStatus(false);
    }

    const arr = Array.from(selectedApp);

    if (index === 0 && arr?.length > 1) {
      arr.pop();
    }

    arr[index] = name;

    setSelected(arr);
  };

  const listHeight = useMemo(() => {
    const height = (window?.innerHeight || 0) - 160;

    const listRequiredHeight =
      // 35 is list item height
      Math.max(groups?.length || 0, profilesNames?.length || 0) * 35;

    if (height && listRequiredHeight) {
      return height >= listRequiredHeight ? 'auto' : `${height}px`;
    }

    return 'auto';
  }, [groups, profilesNames]);

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
              value={filters.search.unwrapOr('')}
              onChange={(e) => handleFilterChange('search', e.target.value)}
              className={styles.searchInput}
              testId="application-search"
            />
          </div>
          <div>
            <div className={styles.headerTitle}>
              <FontAwesomeIcon icon={faSlidersH} /> FILTERS
              <button
                className={styles.resetFilters}
                disabled={
                  filters.profileTypes.isNothing && filters.spyNames.isNothing
                }
                onClick={resetClickableFilters}
              >
                <FontAwesomeIcon icon={faUndo} />
              </button>
            </div>
            <div>
              <div className={styles.filter}>
                <div className={styles.filterName}>Language</div>
                <div className={styles.iconsContainer}>
                  {spyNameValues.map((v) => (
                    <Tooltip placement="top" title={SPY_NAMES_TOOLTIPS[v]}>
                      <button
                        type="button"
                        key={v}
                        data-testid={v}
                        className={cl(styles.icon, {
                          [styles.active]:
                            filters.spyNames
                              .unwrapOr(
                                [] as (SpyNameFirstClassType | 'unknown')[]
                              )
                              .indexOf(v) !== -1,
                        })}
                        onClick={() => handleFilterChange('spyNames', v)}
                      >
                        {SPY_NAMES_ICONS[v]}
                      </button>
                    </Tooltip>
                  ))}
                </div>
              </div>
              <div className={styles.filter}>
                <div className={styles.filterName}>Profile type</div>
                <div className={styles.profileTypesContainer}>
                  {profileTypeValues.map((v) => (
                    <button
                      type="button"
                      key={v}
                      className={cl(styles.profileType, {
                        [styles.active]:
                          filters.profileTypes
                            .unwrapOr([] as string[])
                            .indexOf(v) !== -1,
                      })}
                      onClick={() => handleFilterChange('profileTypes', v)}
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
          isSelected={selectedApp?.[0] === name}
          key={name}
        />
      ))}
      rightSideEl={profilesNames.map((name) => (
        <SelectButton
          name={name}
          onClick={() => onSelect({ index: 1, name })}
          fullList={filteredAppNames}
          isSelected={selectedApp?.[1] === name}
          key={name}
        />
      ))}
    />
  );
};
