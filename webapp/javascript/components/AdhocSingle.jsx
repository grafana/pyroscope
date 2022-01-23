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
  fetchAdhocProfile,
  setAdhocFile,
  setAdhocProfile,
} from '../redux/actions';
import 'react-tabs/style/react-tabs.css';
import adhocStyles from './Adhoc.module.scss';
import ExportData from './ExportData';

function AdhocSingle(props) {
  const { actions, file, profile, flamebearer, isProfileLoading, raw } = props;
  const { setAdhocFile, setAdhocProfile } = actions;

  useEffect(() => {
    actions.fetchAdhocProfiles();
    return actions.abortAdhocFetchProfiles;
  }, []);

  useEffect(() => {
    if (profile) {
      actions.fetchAdhocProfile(profile);
    }
    return actions.abortAdhocFetchProfile;
  }, [profile]);

  return (
    <div className="pyroscope-app">
      <div className="main-wrapper">
        <Box>
          <Tabs>
            <TabList>
              <Tab>Upload</Tab>
              <Tab>Pyroscope data</Tab>
            </TabList>
            <TabPanel>
              <FileUploader
                className={adhocStyles.tabPanel}
                file={file}
                setFile={setAdhocFile}
              />
            </TabPanel>
            <TabPanel>
              <FileList
                className={adhocStyles.tabPanel}
                profile={profile}
                setProfile={setAdhocProfile}
              />
            </TabPanel>
          </Tabs>
          {isProfileLoading && (
            <div className={classNames('spinner-container')}>
              <Spinner color="rgba(255,255,255,0.6)" size="20px" />
            </div>
          )}
          {!isProfileLoading && (
            <FlameGraphRenderer
              flamebearer={flamebearer}
              viewType="single"
              display="both"
              ExportData={<ExportData flamebearer={raw} exportJSON />}
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
  file: state.root.adhocSingle.file,
  raw: state.root.adhocSingle.raw,
  flamebearer: state.root.adhocSingle.flamebearer,
  profile: state.root.adhocSingle.profile,
  isProfileLoading: state.root.adhocSingle.isProfileLoading,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      fetchAdhocProfiles,
      fetchAdhocProfile,
      setAdhocFile,
      setAdhocProfile,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(AdhocSingle);
