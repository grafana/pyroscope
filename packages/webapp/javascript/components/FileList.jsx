import React from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';

import Spinner from 'react-svg-spinner';

import classNames from 'classnames';
import styles from './FileList.module.scss';
import CheckIcon from './CheckIcon';

function FileList(props) {
  const { areProfilesLoading, profiles, profile, setProfile, className } =
    props;

  const isRowSelected = (id) => {
    return profile === id;
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
                <th>Filename</th>
                <th>Date Modified</th>
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
