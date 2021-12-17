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
import { fetchProfiles, fetchLeftProfile, fetchRightProfile, setLeftFile, setLeftProfile, setRightFile, setRightProfile } from '../redux/actions';
import styles from './ComparisonApp.module.css';

function AdhocComparison(props) {
  const { actions, isLeftProfileLoading, isRightProfileLoading, leftFile, leftFlamebearer, leftProfile, rightFile, rightFlamebearer, rightProfile } =
    props;
  const { setLeftFile, setLeftProfile, setRightFile, setRightProfile } = actions;

  useEffect(() => {
    actions.fetchProfiles();
    return actions.abortFetchProfiles;
  }, []);

  useEffect(() => {
    if (leftProfile) {
      actions.fetchLeftProfile(leftProfile);
    }
    return actions.abortFetchLeftProfile;
  }, [leftProfile]);

  useEffect(() => {
    if (rightProfile) {
      actions.fetchRightProfile(rightProfile);
    }
    return actions.abortFetchRightProfile;
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
                <FileList profile={leftProfile} setProfile={setLeftProfile} />
              </TabPanel>
              <TabPanel>
                <FileUploader file={leftFile} setFile={setLeftFile} />
              </TabPanel>
            </Tabs>
            <div
              className={classNames('spinner-container', {
                visible: isLeftProfileLoading,
              })}
            >
              <Spinner color="rgba(255,255,255,0.6)" size="20px" />
            </div>
            <FlameGraphRenderer
              viewType="double"
              viewSide="left"
              flamebearer={leftFlamebearer}
              data-testid="flamegraph-renderer-left"
              display="both"
            />
          </Box>
          <Box className={styles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Pyroscope data</Tab>
                <Tab>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList profile={rightProfile} setProfile={setRightProfile} />
              </TabPanel>
              <TabPanel>
                <FileUploader file={rightFile} setFile={setRightFile} />
              </TabPanel>
            </Tabs>
            <div
              className={classNames('spinner-container', {
                visible: isRightProfileLoading,
              })}
            >
              <Spinner color="rgba(255,255,255,0.6)" size="20px" />
            </div>
            <FlameGraphRenderer
              viewType="double"
              viewSide="right"
              flamebearer={rightFlamebearer}
              data-testid="flamegraph-renderer-right"
              display="both"
            />
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
  leftProfile: state.root.adhocComparison.left.profile,
  isLeftProfileLoading: state.root.adhocComparison.left.isProfileLoading,
  rightFile: state.root.adhocComparison.right.file,
  rightFlamebearer: state.root.adhocComparison.right.flamebearer,
  rightProfile: state.root.adhocComparison.right.profile,
  isRightProfileLoading: state.root.adhocComparison.right.isProfileLoading,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({ fetchProfiles, fetchLeftProfile, fetchRightProfile, setLeftFile, setLeftProfile, setRightFile, setRightProfile }, dispatch),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocComparison);
