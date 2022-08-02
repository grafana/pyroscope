import React, { useState, useMemo } from 'react';
import { Maybe } from '@webapp/util/fp';
import { AllProfiles } from '@webapp/models/adhoc';
import clsx from 'clsx';
import styles from './FileList.module.scss';
import CheckIcon from './CheckIcon';

const dateModifiedColName = 'updatedAt';
const fileNameColName = 'name';
const tableFormat = [
  { name: fileNameColName, label: 'Filename' },
  { name: dateModifiedColName, label: 'Date Modified' },
];

interface FileListProps {
  className?: string;
  profilesList: AllProfiles;
  onProfileSelected: (id: string) => void;
  selectedProfileId: Maybe<string>;
}

function FileList(props: FileListProps) {
  const {
    profilesList: profiles,
    onProfileSelected,
    className,
    selectedProfileId,
  } = props;

  const [sortBy, updateSortBy] = useState(dateModifiedColName);
  const [sortByDirection, setSortByDirection] = useState('desc');

  const isRowSelected = (id: string) => {
    return selectedProfileId.mapOr(false, (profId) => profId === id);
  };

  const updateSortParams = (newSortBy: 'asc' | 'desc') => {
    let dir = sortByDirection;
    if (sortBy === newSortBy) {
      dir = dir === 'asc' ? 'desc' : 'asc';
    } else {
      dir = 'desc';
    }

    updateSortBy(newSortBy);
    setSortByDirection(dir);
  };

  const sortedProfilesIds = useMemo(() => {
    const m = sortByDirection === 'asc' ? 1 : -1;

    let sorted: AllProfiles[number][] = [];

    if (profiles) {
      const filesInfo = Object.values(profiles);

      switch (sortBy) {
        case fileNameColName:
          sorted = filesInfo.sort(
            (a, b) => m * a[sortBy].localeCompare(b[sortBy])
          );
          break;
        case dateModifiedColName:
          sorted = filesInfo.sort(
            (a, b) => m * (new Date(a[sortBy]) - new Date(b[sortBy]))
          );
          break;
        default:
          sorted = filesInfo;
      }
    }

    return sorted.reduce((acc, { id }) => [...acc, id], []);
  }, [profiles, sortBy, sortByDirection]);

  return (
    <>
      <div className={`${styles.tableContainer} ${className}`}>
        <table className={styles.profilesTable} data-testid="table-view">
          <thead>
            <tr>
              {tableFormat.map(({ name, label }) => (
                <th
                  key={name}
                  className={styles.sortable}
                  onClick={() => updateSortParams(name)}
                >
                  {label}
                  <span
                    className={clsx(
                      styles.sortArrow,
                      sortBy === name && styles[sortByDirection]
                    )}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {profiles &&
              sortedProfilesIds.map((id) => (
                <tr
                  key={id}
                  onClick={() => {
                    // Optimize to not reload the same one
                    if (
                      selectedProfileId.isJust &&
                      selectedProfileId.value === id
                    ) {
                      return;
                    }
                    onProfileSelected(id);
                  }}
                  className={`${isRowSelected(id) && styles.rowSelected}`}
                >
                  <td>
                    {profiles[id].name}

                    {isRowSelected(id) && (
                      <CheckIcon className={styles.checkIcon} />
                    )}
                  </td>
                  <td>{profiles[id].updatedAt}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
    </>
  );
}

export default FileList;
