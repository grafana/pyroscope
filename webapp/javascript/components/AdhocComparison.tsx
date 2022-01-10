import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import classNames from 'classnames';
import FileList from './FileList';
import FileUploader from './FileUploader';
import FlameGraphRenderer from './FlameGraph';
import Footer from './Footer';
import {
  fetchAdhocProfiles,
  fetchAdhocLeftProfile,
  fetchAdhocRightProfile,
  setAdhocLeftFile,
  setAdhocLeftProfile,
  setAdhocRightFile,
  setAdhocRightProfile,
} from '../redux/actions';
import 'react-tabs/style/react-tabs.css';
import styles from './ComparisonApp.module.css';
import adhocStyles from './Adhoc.module.scss';

function AdhocComparison(props) {
  const {
    actions,
    isLeftProfileLoading,
    isRightProfileLoading,
    leftFile,
    leftFlamebearer,
    leftProfile,
    rightFile,
    rightFlamebearer,
    rightProfile,
  } = props;
  const {
    setAdhocLeftFile,
    setAdhocLeftProfile,
    setAdhocRightFile,
    setAdhocRightProfile,
  } = actions;

  useEffect(() => {
    actions.fetchAdhocProfiles();
    return actions.abortFetchAdhocProfiles;
  }, []);

  useEffect(() => {
    if (leftProfile) {
      actions.fetchAdhocLeftProfile(leftProfile);
    }
    return actions.abortFetchAdhocLeftProfile;
  }, [leftProfile]);

  useEffect(() => {
    if (rightProfile) {
      actions.fetchAdhocRightProfile(rightProfile);
    }
    return actions.abortFetchAdhocRightProfile;
  }, [rightProfile]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <div
          className="comparison-container"
          data-testid="comparison-container"
        >
          <Box className={styles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Pyroscope data</Tab>
                <Tab>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={leftProfile}
                  setProfile={setAdhocLeftProfile}
                />
              </TabPanel>
              <TabPanel>
                <FileUploader
                  className={adhocStyles.tabPanel}
                  file={leftFile}
                  setFile={setAdhocLeftFile}
                />
              </TabPanel>
            </Tabs>
            {isLeftProfileLoading && (
              <div className={classNames('spinner-container')}>
                <Spinner color="rgba(255,255,255,0.6)" size="20px" />
              </div>
            )}
            {!isLeftProfileLoading && (
              <FlameGraphRenderer
                viewType="double"
                viewSide="left"
                flamebearer={leftFlamebearer}
                data-testid="flamegraph-renderer-left"
                display="both"
              />
            )}
          </Box>
          <Box className={styles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Pyroscope data</Tab>
                <Tab>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={rightProfile}
                  setProfile={setAdhocRightProfile}
                />
              </TabPanel>
              <TabPanel>
                <FileUploader
                  className={adhocStyles.tabPanel}
                  file={rightFile}
                  setFile={setAdhocRightFile}
                />
              </TabPanel>
            </Tabs>
            {isRightProfileLoading && (
              <div className={classNames('spinner-container')}>
                <Spinner color="rgba(255,255,255,0.6)" size="20px" />
              </div>
            )}
            {!isRightProfileLoading && (
              <FlameGraphRenderer
                viewType="double"
                viewSide="right"
                flamebearer={rightFlamebearer}
                data-testid="flamegraph-renderer-right"
                display="both"
              />
            )}
          </Box>
        </div>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  leftFile: state.root.adhocComparison.left.file,
  leftFlamebearer: state.root.adhocComparison.left.flamebearer,
  leftProfile: state.root.adhocShared.left.profile,
  isLeftProfileLoading: state.root.adhocComparison.left.isProfileLoading,
  rightFile: state.root.adhocComparison.right.file,
  rightFlamebearer: state.root.adhocComparison.right.flamebearer,
  rightProfile: state.root.adhocShared.right.profile,
  isRightProfileLoading: state.root.adhocComparison.right.isProfileLoading,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchAdhocProfiles,
      fetchAdhocLeftProfile,
      fetchAdhocRightProfile,
      setAdhocLeftFile,
      setAdhocLeftProfile,
      setAdhocRightFile,
      setAdhocRightProfile,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocComparison);
