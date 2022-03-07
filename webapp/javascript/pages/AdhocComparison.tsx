import React, { useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { bindActionCreators } from 'redux';
import Box from '@ui/Box';
import { Tab, Tabs, TabList, TabPanel } from 'react-tabs';
import Spinner from 'react-svg-spinner';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import classNames from 'classnames';
import FileList from '../components/FileList';
import FileUploader from '../components/FileUploader';
import Footer from '../components/Footer';
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
import adhocStyles from './Adhoc.module.scss';
import adhocComparisonStyles from './AdhocComparison.module.scss';
import useExportToFlamegraphDotCom from '../components/exportToFlamegraphDotCom.hook';
import ExportData from '../components/ExportData';

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
    leftRaw,
    rightRaw,
  } = props;
  const {
    setAdhocLeftFile,
    setAdhocLeftProfile,
    setAdhocRightFile,
    setAdhocRightProfile,
  } = actions;
  const exportToFlamegraphDotComLeftFn = useExportToFlamegraphDotCom(leftRaw);
  const exportToFlamegraphDotComRightFn = useExportToFlamegraphDotCom(rightRaw);

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
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Upload</Tab>
                <Tab>Pyroscope data</Tab>
              </TabList>
              <TabPanel>
                <FileUploader
                  className={adhocStyles.tabPanel}
                  file={leftFile}
                  setFile={setAdhocLeftFile}
                />
              </TabPanel>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={leftProfile}
                  setProfile={setAdhocLeftProfile}
                />
              </TabPanel>
            </Tabs>
            {isLeftProfileLoading && (
              <div className={classNames('spinner-container')}>
                <Spinner color="rgba(255,255,255,0.6)" size="20px" />
              </div>
            )}
            {!isLeftProfileLoading && (
              <FlamegraphRenderer
                viewType="double"
                viewSide="left"
                flamebearer={leftFlamebearer}
                data-testid="flamegraph-renderer-left"
                display="both"
                ExportData={
                  <ExportData
                    flamebearer={leftRaw}
                    exportJSON
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComLeftFn}
                  />
                }
              />
            )}
          </Box>
          <Box className={adhocComparisonStyles.comparisonPane}>
            <Tabs>
              <TabList>
                <Tab>Upload</Tab>
                <Tab>Pyroscope data</Tab>
              </TabList>
              <TabPanel>
                <FileUploader
                  className={adhocStyles.tabPanel}
                  file={rightFile}
                  setFile={setAdhocRightFile}
                />
              </TabPanel>
              <TabPanel>
                <FileList
                  className={adhocStyles.tabPanel}
                  profile={rightProfile}
                  setProfile={setAdhocRightProfile}
                />
              </TabPanel>
            </Tabs>
            {isRightProfileLoading && (
              <div className={classNames('spinner-container')}>
                <Spinner color="rgba(255,255,255,0.6)" size="20px" />
              </div>
            )}
            {!isRightProfileLoading && (
              <FlamegraphRenderer
                viewType="double"
                viewSide="right"
                flamebearer={rightFlamebearer}
                data-testid="flamegraph-renderer-right"
                display="both"
                ExportData={
                  <ExportData
                    flamebearer={rightRaw}
                    exportJSON
                    exportFlamegraphDotCom
                    exportFlamegraphDotComFn={exportToFlamegraphDotComRightFn}
                  />
                }
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
  leftRaw: state.root.adhocComparison.left.raw,
  rightRaw: state.root.adhocComparison.right.raw,
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
