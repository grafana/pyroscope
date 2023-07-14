import React, { useState, useEffect, useMemo } from 'react';
import ModalWithToggle from '@webapp/ui/Modals/ModalWithToggle';
import { App, appFromQuery, appToQuery } from '@webapp/models/app';
import { Query } from '@webapp/models/query';
import cx from 'classnames';
import { SelectButton } from '@phlare/components/AppSelector/SelectButton';
import ogStyles from '@pyroscope/webapp/javascript/components/AppSelector/AppSelector.module.scss';
import styles from '@phlare/components/AppSelector/AppSelector.module.css';

//type App = Omit<OgApp, 'name'>;

interface AppSelectorProps {
  /** Triggered when an app is selected */
  onSelected: (query: Query) => void;

  /** List of all applications */
  apps: App[];

  selectedQuery: Query;
}

// TODO: unify this with public/app/overrides/services/apps.ts
function uniqueByName(apps: App[]) {
  const idFn = (b: App) => b.name;
  const visited = new Set<string>();

  return apps.filter((b) => {
    if (visited.has(idFn(b))) {
      return false;
    }

    visited.add(idFn(b));
    return true;
  });
}

function findAppsWithName(apps: App[], appName: string) {
  return apps.filter((a) => {
    return a.name === appName;
  });
}

function queryToApp(query: Query, apps: App[]) {
  const maybeSelectedApp = appFromQuery(query);
  if (!maybeSelectedApp) {
    return undefined;
  }

  return apps.find(
    (a) =>
      a.__profile_type__ === maybeSelectedApp?.__profile_type__ &&
      a.name === maybeSelectedApp?.name
  );
}

export function AppSelector({
  onSelected,
  apps,
  selectedQuery,
}: AppSelectorProps) {
  const maybeSelectedApp = queryToApp(selectedQuery, apps);
  const [filter, setFilter] = useState('');
  const filteredApps = useMemo(
    () =>
      apps.filter((app) =>
        app.name.toLowerCase().includes(filter.trim().toLowerCase())
      ),
    [apps, filter]
  );
  useEffect(() => {
    setFilter('');
  }, [selectedQuery]);

  return (
    <div className={ogStyles.container}>
      <SelectorModalWithToggler
        apps={filteredApps}
        onSelected={(app) => onSelected(appToQuery(app))}
        selectedApp={maybeSelectedApp}
        filter={filter}
        setFilter={setFilter}
      />
    </div>
  );
}

export const SelectorModalWithToggler = ({
  apps,
  selectedApp,
  onSelected: onSelectedUpstream,
  filter,
  setFilter,
}: {
  apps: App[];
  selectedApp?: App;
  onSelected: (app: App) => void;
  filter: string;
  setFilter: (filter: string) => void;
}) => {
  const onSelected = (app: App) => {
    // Reset state
    setSelectedLeftSide(undefined);

    onSelectedUpstream(app);
  };

  const leftSideApps = uniqueByName(apps);
  const [isModalOpen, setModalOpenStatus] = useState(false);
  const [selectedLeftSide, setSelectedLeftSide] = useState<string>();
  const matchedApps = findAppsWithName(
    apps,
    selectedLeftSide || selectedApp?.name || ''
  );
  const label = 'Select an application';

  // For the left side, it's possible to be selected either via
  // * The current query (ie. just opened the component)
  // * The current "expanded state" (ie. clicked on the left side)
  const isLeftSideSelected = (a: App) => {
    if (selectedLeftSide) {
      return selectedLeftSide === a.name;
    }

    return selectedApp?.name === a.name;
  };

  // For the right side, the only way to be selected is if matches the current query
  // Since clicking on an item sets that app as the current query
  const isRightSideSelected = (a: App) => {
    if (selectedLeftSide) {
      return false;
    }

    return selectedApp?.__profile_type__ === a.__profile_type__;
  };

  const groups = useMemo(() => {
    const allGroups = leftSideApps.map((app) => app.name.split('-')[0]);
    const uniqGroups = Array.from(new Set(allGroups));

    const dedupedUniqGroups = uniqGroups.filter((x) => {
      return !uniqGroups.find((y) => x !== y && y.startsWith(x));
    });

    const groupOrApp = dedupedUniqGroups.map((groupName) => {
      const appNamesEntries = leftSideApps.filter((app) =>
        app.name.startsWith(groupName)
      );

      return appNamesEntries.length > 1 ? groupName : appNamesEntries[0].name;
    });

    return groupOrApp;
  }, [leftSideApps]);

  const listHeight = useMemo(() => {
    const windowHeight = window?.innerHeight || 0;
    const listRequiredHeight = Math.max(groups.length, matchedApps.length) * 35;

    if (windowHeight && listRequiredHeight) {
      return windowHeight >= listRequiredHeight ? 'auto' : `${windowHeight}px`;
    }

    return 'auto';
  }, [groups, matchedApps]);

  return (
    <ModalWithToggle
      isModalOpen={isModalOpen}
      setModalOpenStatus={setModalOpenStatus}
      modalClassName={cx(ogStyles.appSelectorModal, styles.appSelectorModal)}
      customHandleOutsideClick={() => {
        setSelectedLeftSide(undefined);
        setModalOpenStatus(false);
      }}
      modalHeight={listHeight}
      noDataEl={
        !leftSideApps?.length ? (
          <div data-testid="app-selector-no-data" className={ogStyles.noData}>
            No Data
          </div>
        ) : null
      }
      toggleText={
        selectedApp
          ? `${selectedApp?.name}:${selectedApp.__name__}:${selectedApp.__type__}`
          : label
      }
      headerEl={
        <>
          <div className={ogStyles.headerTitle}>{label}</div>
          <input
            type="text"
            placeholder="Search..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className={ogStyles.search}
            data-testid="app-selector-search"
          />
        </>
      }
      leftSideEl={leftSideApps.map((app) => (
        <SelectButton
          name={app.name}
          onClick={() => {
            setSelectedLeftSide(app.name);
          }}
          icon="folder"
          isSelected={isLeftSideSelected(app)}
          key={app.name}
        />
      ))}
      rightSideEl={matchedApps.map((app) => (
        <SelectButton
          name={`${app.__name__}:${app.__type__}`}
          icon="pyroscope"
          onClick={() => onSelected(app)}
          isSelected={isRightSideSelected(app)}
          key={app.__profile_type__}
        />
      ))}
    />
  );
};

export default AppSelector;
