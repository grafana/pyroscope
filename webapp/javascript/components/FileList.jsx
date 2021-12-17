import React from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';

import Spinner from 'react-svg-spinner';

import classNames from 'classnames';
import styles from './FileList.module.scss';

function FileList(props) {
  const { areProfilesLoading, profiles, profile, setProfile } = props;
  return (
    <>
      <div
        className={classNames('spinner-container', {
          visible: areProfilesLoading,
        })}
      >
        <Spinner color="rgba(255,255,255,0.6)" size="20px" />
      </div>
      <div className={styles.tableContainer}>
        <table className="flamegraph-table" data-testid="table-view">
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
                  onClick={() => {
                    console.log('clicked!', id);
                    setProfile(id);
                  }}
                  className={classNames('filelist-row', {
                    selected: profile === id,
                  })}
                >
                  <td>{profiles[id].name}</td>
                  <td>{profiles[id].updated_at}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>
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
