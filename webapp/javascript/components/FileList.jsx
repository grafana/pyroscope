import React, { useState } from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';

import Spinner from 'react-svg-spinner';

import classNames from 'classnames';
import clsx from 'clsx';
import styles from './FileList.module.scss';
import CheckIcon from './CheckIcon';

const dateModifiedColName = 'dateModified';
const tableFormat = [
  { name: 'filename', label: 'Filename' },
  { name: dateModifiedColName, label: 'Date Modified' },
];

function FileList(props) {
  const { areProfilesLoading, profiles, profile, setProfile, className } =
    props;

  const [sortBy, updateSortBy] = useState(dateModifiedColName);
  const [sortByDirection, setSortByDirection] = useState('asc');

  const isRowSelected = (id) => {
    return profile === id;
  };

  const updateSort = (newSortBy) => {
    let dir = sortByDirection;
    if (sortBy === newSortBy) {
      dir = dir === 'asc' ? 'desc' : 'asc';
    } else {
      dir = 'desc';
    }

    updateSortBy(newSortBy);
    setSortByDirection(dir);
  };

  return (
    <>
      {areProfilesLoading && (
        <div className={classNames('spinner-container')}>
          <Spinner color="rgba(255,255,255,0.6)" size="20px" />
        </div>
      )}
      {!areProfilesLoading && (
        <div className={`${styles.tableContainer} ${className}`}>
          <table className={styles.profilesTable} data-testid="table-view">
            <thead>
              <tr>
                {tableFormat.map(({ name, label }) => (
                  <th
                    key={name}
                    className={styles.sortable}
                    onClick={() => updateSort(name)}
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
                Object.keys(profiles).map((id) => (
                  <tr
                    key={id}
                    onClick={() => setProfile(id)}
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
      )}
    </>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({}, dispatch),
});

export default connect(mapStateToProps, mapDispatchToProps)(FileList);
