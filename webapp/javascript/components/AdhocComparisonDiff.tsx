import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import classNames from 'classnames';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import FileList from './FileList';
import Footer from './Footer';
import {
  fetchAdhocProfiles,
  fetchAdhocProfileDiff,
  setAdhocLeftProfile,
  setAdhocRightProfile,
} from '../redux/actions';
import styles from './ComparisonApp.module.css';
import 'react-tabs/style/react-tabs.css';
import adhocStyles from './Adhoc.module.scss';
import useExportToFlamegraphDotCom from './exportToFlamegraphDotCom.hook';
import ExportData from './ExportData';

function AdhocComparisonDiff(props) {
  const {
    actions,
    isProfileLoading,
    flamebearer,
    leftProfile,
    rightProfile,
    raw,
  } = props;
  const { setAdhocLeftProfile, setAdhocRightProfile } = actions;
  const exportToFlamegraphDotComFn = useExportToFlamegraphDotCom(raw);

  useEffect(() => {
    actions.fetchAdhocProfiles();
    return actions.abortFetchAdhocProfiles;
  }, []);

  useEffect(() => {
    if (leftProfile && rightProfile) {
      actions.fetchAdhocProfileDiff(leftProfile, rightProfile);
    }
    return actions.abortFetchAdhocProfileDiff;
  }, [leftProfile, rightProfile]);

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
                <Tab disabled>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={leftProfile}
                  setProfile={setAdhocLeftProfile}
                />
              </TabPanel>
              <TabPanel />
            </Tabs>
          </Box>
          <Box className={styles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Pyroscope data</Tab>
                <Tab disabled>Upload</Tab>
              </TabList>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={rightProfile}
                  setProfile={setAdhocRightProfile}
                />
              </TabPanel>
              <TabPanel />
            </Tabs>
          </Box>
        </div>
        <Box>
          {isProfileLoading && (
            <div className={classNames('spinner-container')}>
              <Spinner color="rgba(255,255,255,0.6)" size="20px" />
            </div>
          )}
          {!isProfileLoading && (
            <FlamegraphRenderer
              display="both"
              viewType="diff"
              flamebearer={flamebearer}
              ExportData={
                <ExportData
                  flamebearer={raw}
                  exportJSON
                  exportFlamegraphDotCom
                  exportFlamegraphDotComFn={exportToFlamegraphDotComFn}
                />
              }
            />
          )}
        </Box>
      </div>
      <Footer />
    </div>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
  raw: state.root.adhocComparisonDiff.raw,
  flamebearer: state.root.adhocComparisonDiff.flamebearer,
  isProfileLoading: state.root.adhocComparisonDiff.isProfileLoading,
  leftProfile: state.root.adhocShared.left.profile,
  rightProfile: state.root.adhocShared.right.profile,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchAdhocProfiles,
      fetchAdhocProfileDiff,
      setAdhocLeftProfile,
      setAdhocRightProfile,
    },
    dispatch
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(AdhocComparisonDiff);
